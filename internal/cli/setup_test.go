package cli

import (
	"fmt"
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
			relativePath: []string{".agents", "skills", "thr", "SKILL.md"},
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
			if !strings.Contains(output, "Installed thr skill for "+tt.displayName+" at "+path) {
				t.Fatalf("unexpected setup output: %q", output)
			}

			content := readFileString(t, path)
			for _, want := range []string{
				"name: thr",
				"description: Use thr for durable memory across coding sessions",
				thrSkillManagedMarker,
				"thr --format json-v2 context",
				"thr --format json-v2 ask",
				"--scope user",
				"thr move",
				"--cwd",
				"repo:<id>",
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

	if !strings.Contains(output, "The thr skill for Codex is already current at "+path) {
		t.Fatalf("unexpected idempotent setup output: %q", output)
	}
	if got := readFileString(t, path); got != agentSkills.ThrSkill {
		t.Fatalf("expected canonical skill content after idempotent setup")
	}
}

func TestSetupCodexIgnoresCODEXHome(t *testing.T) {
	home := setupTempHome(t)
	codexHome := filepath.Join(t.TempDir(), "codex-home")
	t.Setenv("CODEX_HOME", codexHome)
	path := filepath.Join(home, ".agents", "skills", "thr", "SKILL.md")

	output := runRootCommand(t, "setup", "codex")

	if !strings.Contains(output, "Installed thr skill for Codex at "+path) {
		t.Fatalf("unexpected setup output: %q", output)
	}
	if got := readFileString(t, path); got != agentSkills.ThrSkill {
		t.Fatalf("expected canonical skill content at global agent skill path")
	}
	assertPathAbsent(t, filepath.Join(codexHome, "skills", "thr", "SKILL.md"))
}

func TestSetupCodexRefusesUnmanagedSkillWithoutForce(t *testing.T) {
	home := setupTempHome(t)
	path := filepath.Join(home, ".agents", "skills", "thr", "SKILL.md")
	original := "custom codex skill\n"
	writeTestFile(t, path, original)

	err := executeRootCommand("setup", "codex")

	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite existing unmanaged skill") {
		t.Fatalf("expected unmanaged overwrite refusal, got %v", err)
	}
	if got := readFileString(t, path); got != original {
		t.Fatalf("expected unmanaged skill to be preserved, got %q", got)
	}
}

func TestSetupUpdatesRecognizedManagedSkills(t *testing.T) {
	for _, marker := range []string{thrSkillManagedMarkerV1, thrSkillManagedMarker} {
		t.Run(marker, func(t *testing.T) {
			home := setupTempHome(t)
			path := filepath.Join(home, ".claude", "skills", "thr", "SKILL.md")
			writeTestFile(t, path, "---\nname: thr\ndescription: old\n---\n\n"+marker+"\nold\n")

			output := runRootCommand(t, "setup", "claude-code")

			if !strings.Contains(output, "Updated thr skill for Claude Code at "+path) {
				t.Fatalf("unexpected update output: %q", output)
			}
			if got := readFileString(t, path); got != agentSkills.ThrSkill {
				t.Fatalf("expected managed skill to be replaced with canonical content")
			}
		})
	}
}

func TestSetupRefusesUnmanagedSkillWithoutForce(t *testing.T) {
	home := setupTempHome(t)
	path := filepath.Join(home, ".agents", "skills", "thr", "SKILL.md")
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
	path := filepath.Join(home, ".agents", "skills", "thr", "SKILL.md")
	writeTestFile(t, path, "custom user skill\n")

	runRootCommand(t, "setup", "opencode", "--force")

	if got := readFileString(t, path); got != agentSkills.ThrSkill {
		t.Fatalf("expected --force to replace unmanaged skill")
	}
}

func TestSetupOpenCodeAndCodexShareGlobalAgentSkill(t *testing.T) {
	home := setupTempHome(t)
	path := filepath.Join(home, ".agents", "skills", "thr", "SKILL.md")

	runRootCommand(t, "setup", "opencode")
	output := runRootCommand(t, "setup", "codex")

	if !strings.Contains(output, "The thr skill for Codex is already current at "+path) {
		t.Fatalf("unexpected shared setup output: %q", output)
	}
	assertPathAbsent(t, filepath.Join(home, ".config", "opencode", "skills", "thr", "SKILL.md"))
	assertPathAbsent(t, filepath.Join(home, ".codex", "skills", "thr", "SKILL.md"))
}

func TestSetupMigratesLegacyManagedSkills(t *testing.T) {
	for _, command := range []string{"codex", "opencode"} {
		for _, marker := range []string{thrSkillManagedMarkerV1, thrSkillManagedMarker} {
			for _, current := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/current=%t", command, marker, current), func(t *testing.T) {
					home := setupTempHome(t)
					codexHome := filepath.Join(t.TempDir(), "custom-codex")
					t.Setenv("CODEX_HOME", codexHome)
					path := filepath.Join(home, ".agents", "skills", "thr", "SKILL.md")
					if current {
						writeTestFile(t, path, agentSkills.ThrSkill)
					}
					legacyPaths := []string{
						filepath.Join(home, ".codex", "skills", "thr", "SKILL.md"),
						filepath.Join(home, ".config", "opencode", "skills", "thr", "SKILL.md"),
						filepath.Join(codexHome, "skills", "thr", "SKILL.md"),
					}
					for _, legacyPath := range legacyPaths {
						writeTestFile(t, legacyPath, marker+"\nold skill\n")
					}
					assetPath := filepath.Join(filepath.Dir(legacyPaths[0]), "notes.txt")
					writeTestFile(t, assetPath, "supporting file\n")

					output := runRootCommand(t, "setup", command)

					if !strings.Contains(output, "Updated thr skill for") {
						t.Fatalf("expected migration to report an update, got %q", output)
					}
					if got := readFileString(t, path); got != agentSkills.ThrSkill {
						t.Fatal("expected current shared skill after migration")
					}
					for _, legacyPath := range legacyPaths {
						assertPathAbsent(t, legacyPath)
					}
					if got := readFileString(t, assetPath); got != "supporting file\n" {
						t.Fatalf("supporting file changed: %q", got)
					}
					assertPathAbsent(t, filepath.Dir(legacyPaths[1]))
					assertPathAbsent(t, filepath.Dir(legacyPaths[2]))
					if output := runRootCommand(t, "setup", command); !strings.Contains(output, "already current") {
						t.Fatalf("expected idempotent setup after migration, got %q", output)
					}
				})
			}
		}
	}
}

func TestSetupPreservesLegacySkillsWhenInstallFails(t *testing.T) {
	for _, obstacle := range []string{"unmanaged", "symlink", "directory"} {
		t.Run(obstacle, func(t *testing.T) {
			home := setupTempHome(t)
			legacyPath := filepath.Join(home, ".codex", "skills", "thr", "SKILL.md")
			original := thrSkillManagedMarkerV1 + "\nold skill\n"
			writeTestFile(t, legacyPath, original)
			path := filepath.Join(home, ".agents", "skills", "thr", "SKILL.md")
			writeTestFile(t, path, "custom skill\n")
			if obstacle != "unmanaged" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if obstacle == "symlink" {
					if err := os.Symlink(legacyPath, path); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}

			if err := executeRootCommand("setup", "codex"); err == nil {
				t.Fatal("expected replacement installation to fail")
			}
			if got := readFileString(t, legacyPath); got != original {
				t.Fatalf("legacy skill changed after failed installation: %q", got)
			}
		})
	}
}

func TestSetupPreservesUnmanagedAndLinkedLegacySkills(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			home := setupTempHome(t)
			customPath := filepath.Join(home, ".config", "opencode", "skills", "thr", "SKILL.md")
			writeTestFile(t, customPath, "custom skill\n")
			managedPath := filepath.Join(t.TempDir(), "thr", "SKILL.md")
			writeTestFile(t, managedPath, thrSkillManagedMarkerV1+"\nmanaged elsewhere\n")
			linkedPath := filepath.Join(home, ".codex", "skills", "thr", "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(linkedPath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(managedPath, linkedPath); err != nil {
				t.Fatal(err)
			}
			codexHome := t.TempDir()
			t.Setenv("CODEX_HOME", codexHome)
			linkedDir := filepath.Join(codexHome, "skills", "thr")
			if err := os.MkdirAll(filepath.Dir(linkedDir), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Dir(managedPath), linkedDir); err != nil {
				t.Fatal(err)
			}
			args := []string{"setup", "codex"}
			if force {
				args = append(args, "--force")
			}

			runRootCommand(t, args...)

			if got := readFileString(t, customPath); got != "custom skill\n" {
				t.Fatalf("custom legacy skill changed: %q", got)
			}
			if got := readFileString(t, managedPath); got != thrSkillManagedMarkerV1+"\nmanaged elsewhere\n" {
				t.Fatalf("linked skill changed: %q", got)
			}
			for _, path := range []string{linkedPath, linkedDir} {
				if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("expected legacy symlink to remain at %s: %v", path, err)
				}
			}
		})
	}
}

func TestSetupPreservesCanonicalSkillViaCODEXHomeAlias(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(fmt.Sprintf("symlink=%t", linked), func(t *testing.T) {
			home := setupTempHome(t)
			codexHome := filepath.Join(home, ".agents")
			if linked {
				alias := filepath.Join(home, "agents-alias")
				if err := os.Symlink(codexHome, alias); err != nil {
					t.Fatal(err)
				}
				codexHome = alias
			}
			t.Setenv("CODEX_HOME", codexHome)

			runRootCommand(t, "setup", "codex")

			path := filepath.Join(home, ".agents", "skills", "thr", "SKILL.md")
			if got := readFileString(t, path); got != agentSkills.ThrSkill {
				t.Fatal("canonical skill was changed during legacy cleanup")
			}
		})
	}
}

func setupTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")
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
