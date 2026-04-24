package cli

import (
	"database/sql"
	"fmt"

	"github.com/chadiek/thr/internal/config"
	"github.com/chadiek/thr/internal/embed"
	"github.com/chadiek/thr/internal/store"
)

type runtimeDeps struct {
	config   config.Config
	db       *sql.DB
	repo     *store.Repository
	embedder embed.Embedder
}

func initRuntime(dbFlag string, withEmbedder bool) (*runtimeDeps, func(), error) {
	cfg, err := config.Load(dbFlag)
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.EnsureDirs(); err != nil {
		return nil, nil, err
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, nil, err
	}

	deps := &runtimeDeps{
		config: cfg,
		db:     db,
		repo:   store.NewRepository(db),
	}

	cleanup := func() {
		if deps.embedder != nil {
			_ = deps.embedder.Close()
		}
		_ = db.Close()
	}

	if withEmbedder {
		bge, err := embed.NewBGEEmbedder(cfg.ModelCache)
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("initialize embedder: %w", err)
		}
		deps.embedder = bge
	}

	return deps, cleanup, nil
}
