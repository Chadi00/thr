package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsSQLiteURI(t *testing.T) {
	t.Setenv(envDBPath, "file:/tmp/thr.db")

	_, err := Load("")
	if err == nil || !strings.Contains(err.Error(), "filesystem path") {
		t.Fatalf("expected filesystem path error, got %v", err)
	}
}

func TestLoadExpandsTildePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(envDBPath, "~/.cache/thr.db")
	t.Setenv(envModelCache, "~/.cache/thr-models")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DBPath != filepath.Join(home, ".cache", "thr.db") {
		t.Fatalf("unexpected db path: %q", cfg.DBPath)
	}
	if cfg.ModelCache != filepath.Join(home, ".cache", "thr-models") {
		t.Fatalf("unexpected model cache: %q", cfg.ModelCache)
	}

	if err := cfg.EnsureDBDir(); err != nil {
		t.Fatalf("ensure db dir: %v", err)
	}
	if err := cfg.EnsureModelCacheDir(); err != nil {
		t.Fatalf("ensure model cache: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(cfg.DBPath)); err != nil {
		t.Fatalf("stat db dir: %v", err)
	}
	if _, err := os.Stat(cfg.ModelCache); err != nil {
		t.Fatalf("stat model cache: %v", err)
	}
}
