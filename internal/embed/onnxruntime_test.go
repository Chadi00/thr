package embed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveONNXRuntimeLibraryUsesOverride(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, onnxRuntimeLibraryName())
	if err := os.WriteFile(lib, []byte("stub"), 0o600); err != nil {
		t.Fatalf("write stub library: %v", err)
	}
	t.Setenv(onnxRuntimeEnvVar, lib)

	got, err := resolveONNXRuntimeLibrary()
	if err != nil {
		t.Fatalf("resolve override: %v", err)
	}
	if got != lib {
		t.Fatalf("expected %q, got %q", lib, got)
	}
}

func TestResolveONNXRuntimeLibraryRejectsMissingOverride(t *testing.T) {
	missing := filepath.Join(t.TempDir(), onnxRuntimeLibraryName())
	t.Setenv(onnxRuntimeEnvVar, missing)

	_, err := resolveONNXRuntimeLibrary()
	if err == nil {
		t.Fatal("expected missing override to fail")
	}
	if !strings.Contains(err.Error(), onnxRuntimeEnvVar) {
		t.Fatalf("expected error to mention %s, got %v", onnxRuntimeEnvVar, err)
	}
}
