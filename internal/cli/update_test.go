package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateVerifiesAndRunsLatestInstaller(t *testing.T) {
	for _, command := range []string{"bash", "curl", "ssh-keygen"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skip(command + " unavailable")
		}
	}

	prefix := filepath.Join(t.TempDir(), "thr prefix")
	dbPath := filepath.Join(t.TempDir(), "thr.db")
	t.Setenv("THR_INSTALL_PREFIX", prefix)
	t.Setenv("THR_INSTALL_LIB_ONLY", "1")
	t.Setenv("THR_INSTALL_TEST_BASE_URL", "https://example.invalid")
	t.Setenv("BASH_ENV", filepath.Join(t.TempDir(), "must-not-run"))
	t.Setenv("BASH_FUNC_thr_update_test%%", "() { printf compromised; }")

	fixture := t.TempDir()
	installer := []byte("#!/usr/bin/env bash\nif type thr_update_test >/dev/null 2>&1; then function=present; else function=; fi\nprintf 'prefix=%s db=%s skip=%s path=%s lib_only=%s test_url=%s bash_env=%s function=%s\\n' \"$THR_INSTALL_PREFIX\" \"$THR_DB\" \"$THR_INSTALL_SKIP_SKILL_PROMPT\" \"$PATH\" \"${THR_INSTALL_LIB_ONLY:-}\" \"${THR_INSTALL_TEST_BASE_URL:-}\" \"${BASH_ENV:-}\" \"$function\"\n")
	checksum := sha256.Sum256(installer)
	checksums := []byte(fmt.Sprintf("%x  install.sh\n", checksum))
	writeTestFile(t, filepath.Join(fixture, "install.sh"), string(installer))
	writeTestFile(t, filepath.Join(fixture, "checksums.txt"), string(checksums))

	key := filepath.Join(fixture, "signing-key")
	runTestCommand(t, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "thr-update-test", "-f", key)
	runTestCommand(t, "ssh-keygen", "-Y", "sign", "-f", key, "-n", "thr-release", filepath.Join(fixture, "checksums.txt"))
	publicKey, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.FileServer(http.Dir(fixture)))
	defer server.Close()
	originalURL, originalSigner := updateReleaseURL, updateAllowedSigner
	updateReleaseURL = server.URL
	updateAllowedSigner = "thr-release " + strings.TrimSpace(string(publicKey))
	t.Cleanup(func() {
		updateReleaseURL = originalURL
		updateAllowedSigner = originalSigner
	})

	stdout, stderr := runRootCommandStreams(t, "--db", dbPath, "--format", "json-v2", "update")
	for _, want := range []string{"prefix=" + prefix, "db=" + dbPath, "skip=1", "path=" + filepath.Join(prefix, "bin") + string(os.PathListSeparator)} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("installer output missing %q: %q", want, stderr)
		}
	}
	for _, forbidden := range []string{"lib_only=1", "test_url=https://example.invalid", "bash_env=" + os.Getenv("BASH_ENV"), "function=present"} {
		if strings.Contains(stderr, forbidden) {
			t.Fatalf("installer inherited %q: %q", forbidden, stderr)
		}
	}
	if !strings.Contains(stdout, `"command": "software.update"`) || !strings.Contains(stdout, `"status": "updated"`) {
		t.Fatalf("unexpected update JSON: %q", stdout)
	}

	writeTestFile(t, filepath.Join(fixture, "install.sh"), "tampered\n")
	if err := executeRootCommand("update"); err == nil || !strings.Contains(err.Error(), "installer checksum mismatch") {
		t.Fatalf("expected tampered installer rejection, got %v", err)
	}

	tamperedChecksum := sha256.Sum256([]byte("tampered\n"))
	writeTestFile(t, filepath.Join(fixture, "checksums.txt"), fmt.Sprintf("%x  install.sh\n", tamperedChecksum))
	root := NewRootCommand("v1.2.3", "abc123", "2026-04-24T00:00:00Z")
	stdoutBuffer, stderrBuffer := new(bytes.Buffer), new(bytes.Buffer)
	root.SetOut(stdoutBuffer)
	root.SetErr(stderrBuffer)
	root.SetArgs([]string{"--format", "json-v2", "update"})
	executed, err := root.ExecuteContextC(context.Background())
	if err == nil || !strings.Contains(err.Error(), "verify signed release checksums") {
		t.Fatalf("expected tampered signature rejection, got %v", err)
	}
	PrintError(executed, err, stderrBuffer)
	var failure struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal(stderrBuffer.Bytes(), &failure); err != nil {
		t.Fatalf("update failure was not clean JSON: %v: %q", err, stderrBuffer.String())
	}
	if failure.OK || failure.Command != "software.update" {
		t.Fatalf("unexpected update failure envelope: %q", stderrBuffer.String())
	}
}

func runTestCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	if output, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s failed: %v: %s", name, err, output)
	}
}
