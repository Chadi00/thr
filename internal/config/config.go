package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	envDBPath     = "THR_DB"
	envModelCache = "THR_MODEL_CACHE"
	defaultDBName = "thr.db"
)

type Config struct {
	DBPath     string
	ModelCache string
}

func Load(dbFlag string) (Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home dir: %w", err)
	}

	thrHome := filepath.Join(homeDir, ".thr")
	dbPath := expandPath(homeDir, firstNonEmpty(dbFlag, os.Getenv(envDBPath), filepath.Join(thrHome, defaultDBName)))
	if strings.HasPrefix(dbPath, "file:") {
		return Config{}, fmt.Errorf("THR_DB/--db must be a filesystem path, not a SQLite URI: %q", dbPath)
	}
	modelCache := expandPath(homeDir, firstNonEmpty(os.Getenv(envModelCache), filepath.Join(thrHome, "models")))

	return Config{
		DBPath:     dbPath,
		ModelCache: modelCache,
	}, nil
}

func (c Config) EnsureDBDir() error {
	if err := os.MkdirAll(filepath.Dir(c.DBPath), 0o755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}
	return nil
}

func (c Config) EnsureModelCacheDir() error {
	if err := os.MkdirAll(c.ModelCache, 0o755); err != nil {
		return fmt.Errorf("create model cache directory: %w", err)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func expandPath(homeDir string, value string) string {
	if value == "~" {
		return homeDir
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(homeDir, strings.TrimPrefix(value, "~/"))
	}
	return value
}
