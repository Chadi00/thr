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

func TestEnsureDirsUsePrivateModes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := cfg.EnsureDBDir(); err != nil {
		t.Fatalf("ensure db dir: %v", err)
	}
	if err := cfg.EnsureModelCacheDir(); err != nil {
		t.Fatalf("ensure model cache: %v", err)
	}

	assertMode(t, filepath.Dir(cfg.DBPath), 0o700)
	assertMode(t, cfg.ModelCache, 0o700)
}

func TestEnsureDirsHardenExistingModes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(envDBPath, filepath.Join(home, "open", "thr.db"))
	t.Setenv(envModelCache, filepath.Join(home, "models"))

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		t.Fatalf("create db dir: %v", err)
	}
	if err := os.MkdirAll(cfg.ModelCache, 0o755); err != nil {
		t.Fatalf("create model cache: %v", err)
	}
	if err := cfg.EnsureDBDir(); err != nil {
		t.Fatalf("ensure db dir: %v", err)
	}
	if err := cfg.EnsureModelCacheDir(); err != nil {
		t.Fatalf("ensure model cache: %v", err)
	}

	assertMode(t, filepath.Dir(cfg.DBPath), 0o700)
	assertMode(t, cfg.ModelCache, 0o700)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s: got %o want %o", path, got, want)
	}
}
