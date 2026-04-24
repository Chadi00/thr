package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	envDBPath      = "THR_DB"
	envModelCache  = "THR_MODEL_CACHE"
	defaultDBName  = "thr.db"
	defaultTopKAsk = 3
)

type Config struct {
	HomeDir      string
	DBPath       string
	ModelCache   string
	DefaultAskK  int
	DefaultListN int
}

func Load(dbFlag string) (Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home dir: %w", err)
	}

	thrHome := filepath.Join(homeDir, ".thr")
	dbPath := firstNonEmpty(dbFlag, os.Getenv(envDBPath), filepath.Join(thrHome, defaultDBName))
	modelCache := firstNonEmpty(os.Getenv(envModelCache), filepath.Join(thrHome, "models"))

	return Config{
		HomeDir:      thrHome,
		DBPath:       dbPath,
		ModelCache:   modelCache,
		DefaultAskK:  defaultTopKAsk,
		DefaultListN: 100,
	}, nil
}

func (c Config) EnsureDirs() error {
	if err := os.MkdirAll(filepath.Dir(c.DBPath), 0o755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}
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
