package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Chadi00/thr/internal/privacy"
	agentSkills "github.com/Chadi00/thr/skills"
	"github.com/spf13/cobra"
)

const thrSkillManagedMarker = "<!-- thr:managed-skill:v1 -->"

type setupTarget struct {
	name                    string
	displayName             string
	relativeSkillPath       []string
	opencodeCompatiblePaths [][]string
}

type setupStatus string

const (
	setupStatusInstalled setupStatus = "installed"
	setupStatusUpdated   setupStatus = "updated"
	setupStatusCurrent   setupStatus = "current"
	setupStatusSkipped   setupStatus = "skipped"
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
	}

	cmd.AddCommand(
		newSetupTargetCommand(setupTarget{
			name:              "claude-code",
			displayName:       "Claude Code",
			relativeSkillPath: []string{".claude", "skills", "thr", "SKILL.md"},
		}),
		newSetupTargetCommand(setupTarget{
			name:              "opencode",
			displayName:       "OpenCode",
			relativeSkillPath: []string{".config", "opencode", "skills", "thr", "SKILL.md"},
			opencodeCompatiblePaths: [][]string{
				{".claude", "skills", "thr", "SKILL.md"},
				{".agents", "skills", "thr", "SKILL.md"},
			},
		}),
		newSetupTargetCommand(setupTarget{
			name:              "codex",
			displayName:       "Codex",
			relativeSkillPath: []string{".agents", "skills", "thr", "SKILL.md"},
		}),
	)

	return cmd
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

	targetPath := filepath.Join(append([]string{homeDir}, target.relativeSkillPath...)...)
	if target.name == "opencode" {
		if exists, err := pathExists(targetPath); err != nil {
			return setupResult{}, err
		} else if !exists {
			for _, relativePath := range target.opencodeCompatiblePaths {
				compatiblePath := filepath.Join(append([]string{homeDir}, relativePath...)...)
				managed, err := isManagedSkillFile(compatiblePath)
				if err != nil {
					return setupResult{}, err
				}
				if managed {
					return setupResult{status: setupStatusSkipped, path: compatiblePath}, nil
				}
			}
		}
	}

	status, err := installSkillFile(targetPath, []byte(agentSkills.ThrSkill), force)
	if err != nil {
		return setupResult{}, err
	}
	return setupResult{status: status, path: targetPath}, nil
}

func printSetupResult(cmd *cobra.Command, target setupTarget, result setupResult) {
	switch result.status {
	case setupStatusInstalled:
		fmt.Fprintf(cmd.OutOrStdout(), "installed thr skill for %s at %s\n", target.displayName, result.path)
	case setupStatusUpdated:
		fmt.Fprintf(cmd.OutOrStdout(), "updated thr skill for %s at %s\n", target.displayName, result.path)
	case setupStatusCurrent:
		fmt.Fprintf(cmd.OutOrStdout(), "thr skill already installed for %s at %s\n", target.displayName, result.path)
	case setupStatusSkipped:
		fmt.Fprintf(cmd.OutOrStdout(), "%s already discovers the managed thr skill at %s; no OpenCode-specific install needed\n", target.displayName, result.path)
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
	if !bytes.Contains(existing, []byte(thrSkillManagedMarker)) && !force {
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

func pathExists(path string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect path %s: %w", path, err)
	}
	return true, nil
}

func isManagedSkillFile(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect compatible skill %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read compatible skill %s: %w", path, err)
	}
	return bytes.Contains(content, []byte(thrSkillManagedMarker)), nil
}
