package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/privacy"
	agentSkills "github.com/Chadi00/thr/skills"
	"github.com/spf13/cobra"
)

const (
	thrSkillManagedMarkerV1 = "<!-- thr:managed-skill:v1 -->"
	thrSkillManagedMarker   = "<!-- thr:managed-skill:v2 -->"
)

type setupTarget struct {
	name              string
	displayName       string
	relativeSkillPath []string
}

type setupStatus string

const (
	setupStatusInstalled setupStatus = "installed"
	setupStatusUpdated   setupStatus = "updated"
	setupStatusCurrent   setupStatus = "current"
)

type setupResult struct {
	status setupStatus
	path   string
}

func newSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install thr integrations for coding agents",
		Long:  "Install the thr Agent Skill for supported coding agents.",
		Args:  cobra.NoArgs,
	}

	for _, target := range setupTargets() {
		cmd.AddCommand(newSetupTargetCommand(target))
	}

	return cmd
}

func setupTargets() []setupTarget {
	return []setupTarget{
		{
			name:              "claude-code",
			displayName:       "Claude Code",
			relativeSkillPath: []string{".claude", "skills", "thr", "SKILL.md"},
		},
		{
			name:              "opencode",
			displayName:       "OpenCode",
			relativeSkillPath: []string{".agents", "skills", "thr", "SKILL.md"},
		},
		{
			name:              "codex",
			displayName:       "Codex",
			relativeSkillPath: []string{".agents", "skills", "thr", "SKILL.md"},
		},
	}
}

func newSetupTargetCommand(target setupTarget) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   target.name,
		Short: fmt.Sprintf("Install the thr skill for %s", target.displayName),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := installSetupTarget(target, force)
			if err != nil {
				return err
			}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "setup."+target.name, independentSelection(cmd), map[string]any{"status": result.status, "path": result.path}, nil)
			}
			printSetupResult(cmd, target, result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Replace an existing unmanaged thr skill")

	return cmd
}

func installSetupTarget(target setupTarget, force bool) (setupResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return setupResult{}, fmt.Errorf("resolve home dir: %w", err)
	}

	targetPath := setupTargetPath(homeDir, target)

	status, err := installSkillFile(targetPath, []byte(agentSkills.ThrSkill), force)
	if err != nil {
		return setupResult{}, err
	}
	removed, err := removeLegacySkills(homeDir, target, targetPath)
	if err != nil {
		return setupResult{}, err
	}
	if removed {
		status = setupStatusUpdated
	}
	return setupResult{status: status, path: targetPath}, nil
}

func setupTargetPath(homeDir string, target setupTarget) string {
	return filepath.Join(append([]string{homeDir}, target.relativeSkillPath...)...)
}

func legacySkillPaths(homeDir string, target setupTarget) []string {
	if target.name != "codex" && target.name != "opencode" {
		return nil
	}
	paths := []string{
		filepath.Join(homeDir, ".codex", "skills", "thr", "SKILL.md"),
		filepath.Join(homeDir, ".config", "opencode", "skills", "thr", "SKILL.md"),
	}
	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
		paths = append(paths, filepath.Join(codexHome, "skills", "thr", "SKILL.md"))
	}
	return paths
}

// Only retire managed copies after the canonical replacement is installed.
func removeLegacySkills(homeDir string, target setupTarget, targetPath string) (bool, error) {
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		return false, fmt.Errorf("inspect installed skill %s: %w", targetPath, err)
	}
	removed := false
	for _, path := range legacySkillPaths(homeDir, target) {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return removed, fmt.Errorf("inspect legacy skill %s: %w", path, err)
		}
		if !info.Mode().IsRegular() || os.SameFile(info, targetInfo) {
			continue
		}
		dir := filepath.Dir(path)
		dirInfo, err := os.Lstat(dir)
		if err != nil {
			return removed, fmt.Errorf("inspect legacy skill directory %s: %w", dir, err)
		}
		if dirInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return removed, fmt.Errorf("read legacy skill %s: %w", path, err)
		}
		if !skillIsManaged(content) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return removed, fmt.Errorf("remove legacy skill %s: %w", path, err)
		}
		removed = true
		// Remove the old skill directory only when empty; preserve supporting files.
		_ = os.Remove(dir)
	}
	return removed, nil
}

func skillIsManaged(content []byte) bool {
	return bytes.Contains(content, []byte(thrSkillManagedMarkerV1)) || bytes.Contains(content, []byte(thrSkillManagedMarker))
}

func printSetupResult(cmd *cobra.Command, target setupTarget, result setupResult) {
	switch result.status {
	case setupStatusInstalled:
		fmt.Fprintf(cmd.OutOrStdout(), "Installed thr skill for %s at %s.\n", target.displayName, output.SanitizeInline(result.path))
	case setupStatusUpdated:
		fmt.Fprintf(cmd.OutOrStdout(), "Updated thr skill for %s at %s.\n", target.displayName, output.SanitizeInline(result.path))
	case setupStatusCurrent:
		fmt.Fprintf(cmd.OutOrStdout(), "The thr skill for %s is already current at %s.\n", target.displayName, output.SanitizeInline(result.path))
	}
}

func installSkillFile(path string, content []byte, force bool) (setupStatus, error) {
	if err := privacy.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return "", err
	}

	info, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect existing skill %s: %w", path, err)
		}
		if err := writeFileAtomic(path, content); err != nil {
			return "", err
		}
		return setupStatusInstalled, nil
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing to replace symlink at %s", path)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("refusing to replace non-regular file at %s", path)
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read existing skill %s: %w", path, err)
	}
	if bytes.Equal(existing, content) {
		if err := os.Chmod(path, privacy.PrivateFileMode); err != nil {
			return "", fmt.Errorf("harden existing skill %s: %w", path, err)
		}
		return setupStatusCurrent, nil
	}
	if !skillIsManaged(existing) && !force {
		return "", fmt.Errorf("refusing to overwrite existing unmanaged skill at %s; rerun with --force to replace it", path)
	}

	if err := writeFileAtomic(path, content); err != nil {
		return "", err
	}
	return setupStatusUpdated, nil
}

func writeFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".SKILL.md.tmp-*")
	if err != nil {
		return fmt.Errorf("create temp skill in %s: %w", dir, err)
	}
	tempPath := file.Name()
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temp skill %s: %w", tempPath, err)
	}
	if err := file.Chmod(privacy.PrivateFileMode); err != nil {
		_ = file.Close()
		return fmt.Errorf("harden temp skill %s: %w", tempPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temp skill %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("install skill %s: %w", path, err)
	}
	keepTemp = true
	if err := os.Chmod(path, privacy.PrivateFileMode); err != nil {
		return fmt.Errorf("harden skill %s: %w", path, err)
	}
	return nil
}

func managedSkillWarnings() []output.Warning {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	warnings := make([]output.Warning, 0)
	seen := map[string]bool{}
	for _, target := range setupTargets() {
		path := setupTargetPath(home, target)
		if seen[path] {
			continue
		}
		seen[path] = true
		content, err := os.ReadFile(path)
		if err != nil || bytes.Equal(content, []byte(agentSkills.ThrSkill)) {
			continue
		}
		if skillIsManaged(content) {
			warnings = append(warnings, output.Warning{
				Code: "managed_skill_outdated", Message: "An installed managed thr skill is outdated.",
				Details: map[string]any{"path": path, "suggested_command": "thr setup " + target.name},
			})
		}
	}
	return warnings
}
