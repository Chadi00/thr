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
	if err := cfg.HardenDBDirIfExists(); err != nil {
		return nil, nil, err
	}
	db, err := store.OpenExisting(cfg.DBPath)
	if err != nil {
		return nil, nil, friendlyStoreError(err)
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
	if err := cfg.HardenDBDirIfExists(); err != nil {
		return nil, nil, err
	}
	db, err := store.OpenExisting(cfg.DBPath)
	if err != nil {
		return nil, nil, friendlyStoreError(err)
	}
	if err := cfg.HardenModelCacheIfExists(); err != nil {
		_ = db.Close()
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
	if err := cfg.HardenModelCacheIfExists(); err != nil {
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
	if err := cfg.HardenModelCacheIfExists(); err != nil {
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

func initExistingWriteRuntimeWithEmbedder(dbFlag string, showEmbedDownload bool) (*runtimeDeps, func(), error) {
	cfg, err := config.Load(dbFlag)
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.HardenDBDirIfExists(); err != nil {
		return nil, nil, err
	}
	db, err := store.OpenExistingWritable(cfg.DBPath)
	if err != nil {
		return nil, nil, friendlyStoreError(err)
	}
	if err := cfg.HardenModelCacheIfExists(); err != nil {
		_ = db.Close()
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

func initPrefetchRuntime(dbFlag string, showEmbedDownload bool) (*runtimeDeps, func(), error) {
	cfg, err := config.Load(dbFlag)
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.HardenModelCacheIfExists(); err != nil {
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

func friendlyStoreError(err error) error {
	if store.IsMigrationRequired(err) {
		return fmt.Errorf("thr needs to update its local data store, but the database is not writable")
	}
	return err
}

func activeEmbeddingIdentity() store.EmbeddingIdentity {
	identity := embed.ActiveModelIdentityValue()
	return store.EmbeddingIdentity{
		ModelID:        identity.ModelID,
		ModelRevision:  identity.ModelRevision,
		ManifestSHA256: identity.ManifestSHA256,
		Dimension:      identity.Dimension,
	}
}
