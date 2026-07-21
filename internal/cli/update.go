package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	updateReleaseURL    = "https://github.com/Chadi00/thr/releases/latest/download"
	updateAllowedSigner = "thr-release ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAXr9HFt+bOkFt6Hx9xC5z/KpwBL0Y5RDonM1eqErPKl thr-release"
)

func newUpdateCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update thr to the latest release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix, err := currentInstallPrefix()
			if err != nil {
				return err
			}
			if err := runLatestInstaller(cmd, prefix, *dbPath); err != nil {
				return err
			}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "software.update", independentSelection(cmd), map[string]any{"status": "updated", "install_prefix": prefix}, nil)
			}
			return nil
		},
	}
}

func runLatestInstaller(cmd *cobra.Command, prefix string, dbPath string) error {
	var diagnostics bytes.Buffer
	commandOutput := cmd.ErrOrStderr()
	if isJSONV2Output(cmd) {
		commandOutput = &diagnostics
	}
	commandError := func(action string, err error) error {
		if message := strings.TrimSpace(diagnostics.String()); message != "" {
			return fmt.Errorf("%s: %w: %s", action, err, message)
		}
		return fmt.Errorf("%s: %w", action, err)
	}

	tempDir, err := os.MkdirTemp("", "thr-update-*")
	if err != nil {
		return fmt.Errorf("create update directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	for _, name := range []string{"install.sh", "checksums.txt", "checksums.txt.sig"} {
		download := exec.CommandContext(cmd.Context(), "curl", "-fsSL", updateReleaseURL+"/"+name, "-o", filepath.Join(tempDir, name))
		download.Stderr = commandOutput
		if err := download.Run(); err != nil {
			return commandError("download latest thr "+name, err)
		}
	}

	allowedSigners := filepath.Join(tempDir, "allowed_signers")
	if err := os.WriteFile(allowedSigners, []byte(updateAllowedSigner+"\n"), 0o600); err != nil {
		return fmt.Errorf("write release signer: %w", err)
	}
	checksumsPath := filepath.Join(tempDir, "checksums.txt")
	checksums, err := os.Open(checksumsPath)
	if err != nil {
		return fmt.Errorf("open release checksums: %w", err)
	}
	verify := exec.CommandContext(cmd.Context(), "ssh-keygen", "-Y", "verify", "-f", allowedSigners, "-I", "thr-release", "-n", "thr-release", "-s", filepath.Join(tempDir, "checksums.txt.sig"))
	verify.Stdin = checksums
	verify.Stdout = io.Discard
	verify.Stderr = commandOutput
	verifyErr := verify.Run()
	closeErr := checksums.Close()
	if verifyErr != nil {
		return commandError("verify signed release checksums", verifyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close release checksums: %w", closeErr)
	}

	checksumData, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("read release checksums: %w", err)
	}
	expected := ""
	for _, line := range strings.Split(string(checksumData), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == "install.sh" {
			expected = fields[0]
			break
		}
	}
	if _, err := hex.DecodeString(expected); len(expected) != sha256.Size*2 || err != nil {
		return fmt.Errorf("release checksums do not contain a valid install.sh checksum")
	}
	installerPath := filepath.Join(tempDir, "install.sh")
	installerData, err := os.ReadFile(installerPath)
	if err != nil {
		return fmt.Errorf("read thr installer: %w", err)
	}
	actual := sha256.Sum256(installerData)
	if hex.EncodeToString(actual[:]) != expected {
		return fmt.Errorf("thr installer checksum mismatch")
	}

	installer := exec.CommandContext(cmd.Context(), "bash", installerPath)
	installer.Stdout = cmd.OutOrStdout()
	if isJSONV2Output(cmd) {
		installer.Stdout = commandOutput
	}
	installer.Stderr = commandOutput
	installer.Env = updateEnvironment(prefix, dbPath)
	if err := installer.Run(); err != nil {
		return commandError("update thr", err)
	}
	if diagnostics.Len() > 0 {
		_, _ = io.Copy(cmd.ErrOrStderr(), &diagnostics)
	}
	return nil
}

func updateEnvironment(prefix string, dbPath string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if strings.HasPrefix(name, "THR_INSTALL_") || strings.HasPrefix(name, "BASH_FUNC_") || name == "BASH_ENV" || name == "BASHOPTS" || name == "SHELLOPTS" || (name == "THR_DB" && dbPath != "") {
			continue
		}
		environment = append(environment, value)
	}
	environment = append(environment, "THR_INSTALL_PREFIX="+prefix, "THR_INSTALL_SKIP_SKILL_PROMPT=1")
	if dbPath != "" {
		environment = append(environment, "THR_DB="+dbPath)
	}
	return environment
}

func currentInstallPrefix() (string, error) {
	if prefix := os.Getenv("THR_INSTALL_PREFIX"); prefix != "" {
		return prefix, nil
	}

	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate thr executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("resolve thr executable: %w", err)
	}
	binDir := filepath.Dir(executable)
	if filepath.Base(binDir) != "bin" {
		return "", fmt.Errorf("cannot determine the thr install prefix from %s; set THR_INSTALL_PREFIX and retry", executable)
	}
	return filepath.Dir(binDir), nil
}
