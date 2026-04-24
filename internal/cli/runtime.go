package cli

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Chadi00/thr/internal/config"
	"github.com/Chadi00/thr/internal/embed"
	"github.com/Chadi00/thr/internal/store"
)

type runtimeDeps struct {
	config   config.Config
	repo     *store.Repository
	embedder embed.Embedder
}

func initReadRuntime(dbFlag string) (*runtimeDeps, func(), error) {
	cfg, err := config.Load(dbFlag)
	if err != nil {
		return nil, nil, err
	}
	db, err := store.OpenExisting(cfg.DBPath)
	if err != nil {
		return nil, nil, err
	}
	deps := &runtimeDeps{
		config: cfg,
		repo:   store.NewRepository(db),
	}
	return deps, cleanupRuntime(deps, db), nil
}

func initReadRuntimeWithEmbedder(dbFlag string, showEmbedDownload bool) (*runtimeDeps, func(), error) {
	cfg, err := config.Load(dbFlag)
	if err != nil {
		return nil, nil, err
	}
	db, err := store.OpenExisting(cfg.DBPath)
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.EnsureModelCacheDir(); err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	bge, err := embed.NewBGEEmbedder(cfg.ModelCache, showEmbedDownload)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("initialize embedder: %w", err)
	}
	deps := &runtimeDeps{
		config:   cfg,
		repo:     store.NewRepository(db),
		embedder: bge,
	}
	return deps, cleanupRuntime(deps, db), nil
}

func initWriteRuntime(dbFlag string) (*runtimeDeps, func(), error) {
	cfg, err := config.Load(dbFlag)
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.EnsureDBDir(); err != nil {
		return nil, nil, err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, nil, err
	}
	deps := &runtimeDeps{
		config: cfg,
		repo:   store.NewRepository(db),
	}
	return deps, cleanupRuntime(deps, db), nil
}

func initWriteRuntimeWithEmbedder(dbFlag string, showEmbedDownload bool) (*runtimeDeps, func(), error) {
	cfg, err := config.Load(dbFlag)
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.EnsureDBDir(); err != nil {
		return nil, nil, err
	}
	if err := cfg.EnsureModelCacheDir(); err != nil {
		return nil, nil, err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, nil, err
	}
	bge, err := embed.NewBGEEmbedder(cfg.ModelCache, showEmbedDownload)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("initialize embedder: %w", err)
	}
	deps := &runtimeDeps{
		config:   cfg,
		repo:     store.NewRepository(db),
		embedder: bge,
	}
	return deps, cleanupRuntime(deps, db), nil
}

func initPrefetchRuntime(dbFlag string, showEmbedDownload bool) (*runtimeDeps, func(), error) {
	cfg, err := config.Load(dbFlag)
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.EnsureModelCacheDir(); err != nil {
		return nil, nil, err
	}
	bge, err := embed.NewBGEEmbedder(cfg.ModelCache, showEmbedDownload)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize embedder: %w", err)
	}
	deps := &runtimeDeps{config: cfg, embedder: bge}
	return deps, cleanupRuntime(deps, nil), nil
}

func cleanupRuntime(deps *runtimeDeps, db *sql.DB) func() {
	return func() {
		if deps.embedder != nil {
			_ = deps.embedder.Close()
		}
		if db != nil {
			_ = db.Close()
		}
	}
}

func isMissingDatabase(err error) bool {
	return errors.Is(err, store.ErrDatabaseNotFound)
}
