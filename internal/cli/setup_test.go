package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentSkills "github.com/Chadi00/thr/skills"
)

func TestSetupCommandsInstallAgentSkills(t *testing.T) {
	tests := []struct {
		command      string
		displayName  string
		relativePath []string
	}{
		{
			command:      "claude-code",
			displayName:  "Claude Code",
			relativePath: []string{".claude", "skills", "thr", "SKILL.md"},
		},
		{
			command:      "opencode",
			displayName:  "OpenCode",
			relativePath: []string{".config", "opencode", "skills", "thr", "SKILL.md"},
		},
		{
			command:      "codex",
			displayName:  "Codex",
			relativePath: []string{".agents", "skills", "thr", "SKILL.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			home := setupTempHome(t)
			modelCache := filepath.Join(t.TempDir(), "models")
			t.Setenv("THR_MODEL_CACHE", modelCache)
			path := filepath.Join(append([]string{home}, tt.relativePath...)...)

			output := runRootCommand(t, "setup", tt.command)
			if !strings.Contains(output, "installed thr skill for "+tt.displayName+" at "+path) {
				t.Fatalf("unexpected setup output: %q", output)
			}

			content := readFileString(t, path)
			for _, want := range []string{
				"name: thr",
				"description: Use thr to retrieve and maintain durable local memories",
				thrSkillManagedMarker,
				"thr ask --json",
				"thr search --json",
				"thr index",
			} {
				if !strings.Contains(content, want) {
					t.Fatalf("expected installed skill to contain %q, got:\n%s", want, content)
				}
			}
			assertSetupPathMode(t, path, 0o600)
			assertSetupPathMode(t, filepath.Dir(path), 0o700)
			assertPathAbsent(t, filepath.Join(home, ".thr"))
			assertPathAbsent(t, modelCache)
		})
	}
}

func TestSetupCommandIsIdempotent(t *testing.T) {
	home := setupTempHome(t)
	path := filepath.Join(home, ".agents", "skills", "thr", "SKILL.md")

	runRootCommand(t, "setup", "codex")
	output := runRootCommand(t, "setup", "codex")

	if !strings.Contains(output, "thr skill already installed for Codex at "+path) {
		t.Fatalf("unexpected idempotent setup output: %q", output)
	}
	if got := readFileString(t, path); got != agentSkills.ThrSkill {
		t.Fatalf("expected canonical skill content after idempotent setup")
	}
}

func TestSetupUpdatesManagedSkill(t *testing.T) {
	home := setupTempHome(t)
	path := filepath.Join(home, ".claude", "skills", "thr", "SKILL.md")
	writeTestFile(t, path, "---\nname: thr\ndescription: old\n---\n\n"+thrSkillManagedMarker+"\nold\n")

	output := runRootCommand(t, "setup", "claude-code")

	if !strings.Contains(output, "updated thr skill for Claude Code at "+path) {
		t.Fatalf("unexpected update output: %q", output)
	}
	if got := readFileString(t, path); got != agentSkills.ThrSkill {
		t.Fatalf("expected managed skill to be replaced with canonical content")
	}
}

func TestSetupRefusesUnmanagedSkillWithoutForce(t *testing.T) {
	home := setupTempHome(t)
	path := filepath.Join(home, ".config", "opencode", "skills", "thr", "SKILL.md")
	original := "custom user skill\n"
	writeTestFile(t, path, original)

	err := executeRootCommand("setup", "opencode")

	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite existing unmanaged skill") {
		t.Fatalf("expected unmanaged overwrite refusal, got %v", err)
	}
	if got := readFileString(t, path); got != original {
		t.Fatalf("expected unmanaged skill to be preserved, got %q", got)
	}
}

func TestSetupForceReplacesUnmanagedSkill(t *testing.T) {
	home := setupTempHome(t)
	path := filepath.Join(home, ".config", "opencode", "skills", "thr", "SKILL.md")
	writeTestFile(t, path, "custom user skill\n")

	runRootCommand(t, "setup", "opencode", "--force")

	if got := readFileString(t, path); got != agentSkills.ThrSkill {
		t.Fatalf("expected --force to replace unmanaged skill")
	}
}

func TestSetupOpenCodeSkipsWhenCompatibleManagedSkillExists(t *testing.T) {
	home := setupTempHome(t)
	compatiblePath := filepath.Join(home, ".agents", "skills", "thr", "SKILL.md")
	opencodePath := filepath.Join(home, ".config", "opencode", "skills", "thr", "SKILL.md")
	writeTestFile(t, compatiblePath, agentSkills.ThrSkill)

	output := runRootCommand(t, "setup", "opencode")

	if !strings.Contains(output, "OpenCode already discovers the managed thr skill at "+compatiblePath) {
		t.Fatalf("unexpected OpenCode skip output: %q", output)
	}
	assertPathAbsent(t, opencodePath)
}

func setupTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create test dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertSetupPathMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s: got %o want %o", path, got, want)
	}
}
