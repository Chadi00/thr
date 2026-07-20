package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Chadi00/thr/internal/config"
	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/repoctx"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

type commandError struct {
	Code             string
	Message          string
	SuggestedCommand string
	Cause            error
}

func (e *commandError) Error() string { return e.Message }
func (e *commandError) Unwrap() error { return e.Cause }

type resolvedSelection struct {
	Mode        string
	Requested   []string
	Scopes      []domain.Scope
	CWD         string
	Database    store.DatabaseInfo
	Resolution  store.RepositoryResolution
	Observation *repoctx.Observation
	Migration   *store.MigrationResult
	Warnings    []output.Warning
}

type selectionContextKey struct{}

func rememberSelection(cmd *cobra.Command, selection resolvedSelection) {
	cmd.SetContext(context.WithValue(cmd.Context(), selectionContextKey{}, selection))
}

func (s resolvedSelection) IDs() []string {
	ids := make([]string, len(s.Scopes))
	for i, scope := range s.Scopes {
		ids[i] = scope.ID
	}
	return ids
}

func resolveReadRuntime(cmd *cobra.Command, dbFlag string, selectors []string, all bool) (*runtimeDeps, resolvedSelection, func(), error) {
	selection, cfg, err := prepareSelection(cmd, dbFlag, selectors, all, false)
	if err != nil {
		return nil, selection, nil, err
	}
	if all {
		selection.Mode = "all"
	} else if len(selectors) > 0 {
		selection.Mode = "explicit"
	} else {
		selection.Mode = "automatic_read"
	}
	rememberSelection(cmd, selection)
	if selection.Database.Status == store.DatabaseMissing {
		if err := resolveMissingRead(&selection, selectors, all); err != nil {
			return nil, selection, nil, err
		}
		rememberSelection(cmd, selection)
		return &runtimeDeps{config: cfg}, selection, func() {}, nil
	}
	db, err := store.OpenExisting(cfg.DBPath)
	if err != nil {
		return nil, selection, nil, friendlyStoreError(err)
	}
	deps := &runtimeDeps{config: cfg, repo: store.NewRepository(db)}
	cleanup := cleanupRuntime(deps, db)
	if err := resolveReadScopes(cmd, deps.repo, &selection, selectors, all); err != nil {
		cleanup()
		return nil, selection, nil, err
	}
	rememberSelection(cmd, selection)
	return deps, selection, cleanup, nil
}

func resolveWriteRuntime(cmd *cobra.Command, dbFlag, selector string) (*runtimeDeps, resolvedSelection, func(), error) {
	selectors := []string{}
	if selector != "" {
		selectors = append(selectors, selector)
	}
	selection, cfg, err := prepareSelection(cmd, dbFlag, selectors, false, true)
	if err != nil {
		return nil, selection, nil, err
	}
	if selector == "" {
		selection.Mode = "automatic_write"
	} else {
		selection.Mode = "explicit"
	}
	rememberSelection(cmd, selection)
	if selector != "" && selector != "user" && selector != "repo" && selection.Database.Status == store.DatabaseMissing {
		return nil, selection, nil, scopeNotFound(selector)
	}
	if selector == "repo" || selector == "" {
		if selection.Observation == nil {
			return nil, selection, nil, writeScopeUnresolved()
		}
	}
	if err := cfg.EnsureDBDir(); err != nil {
		return nil, selection, nil, err
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, selection, nil, friendlyStoreError(err)
	}
	deps := &runtimeDeps{config: cfg, repo: store.NewRepository(db)}
	cleanup := cleanupRuntime(deps, db)
	selection.Database = store.DatabaseInfo{Path: cfg.DBPath, Status: store.DatabaseCompatible}
	switch selector {
	case "user":
		scope, err := deps.repo.GetScope(cmd.Context(), "user")
		if err != nil {
			cleanup()
			return nil, selection, nil, err
		}
		selection.Mode, selection.Scopes = "explicit", []domain.Scope{scope}
	case "", "repo":
		resolution, err := deps.repo.ResolveRepository(cmd.Context(), *selection.Observation)
		if err != nil {
			cleanup()
			return nil, selection, nil, mapRepositoryError(err)
		}
		selection.Resolution = resolution
		selection.Mode = "automatic_write"
		if selector == "repo" {
			selection.Mode = "explicit"
		}
		if resolution.Scope != nil {
			selection.Scopes = []domain.Scope{*resolution.Scope}
		}
	default:
		scope, err := deps.repo.GetScope(cmd.Context(), selector)
		if err != nil {
			cleanup()
			return nil, selection, nil, scopeNotFound(selector)
		}
		selection.Mode, selection.Scopes = "explicit", []domain.Scope{scope}
	}
	rememberSelection(cmd, selection)
	return deps, selection, cleanup, nil
}

func resolveExactRuntime(cmd *cobra.Command, dbFlag string, writable bool) (*runtimeDeps, resolvedSelection, func(), error) {
	cfg, err := config.Load(dbFlag)
	selection := resolvedSelection{}
	if err != nil {
		return nil, selection, nil, err
	}
	cwd, _ := cmd.Flags().GetString("cwd")
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	selection.CWD, _ = filepath.Abs(cwd)
	selection.Database, err = store.InspectOperationalDatabase(cfg.DBPath)
	if err != nil {
		return nil, selection, nil, err
	}
	if selection.Database.Status == store.DatabaseMissing {
		return nil, selection, nil, store.ErrDatabaseNotFound
	}
	if selection.Database.Status == store.DatabaseIncompatible {
		return nil, selection, nil, &commandError{Code: "database_version_incompatible", Message: "the selected database format is incompatible"}
	}
	if selection.Database.Status == store.DatabaseMigrationRequired {
		result, err := store.MigratePath(cmd.Context(), cfg.DBPath)
		if err != nil {
			return nil, selection, nil, &commandError{Code: "database_migration_required", Message: "the selected database requires migration", SuggestedCommand: "thr migrate", Cause: err}
		}
		selection.Migration = &result
		selection.Database.Status = store.DatabaseCompatible
	}
	rememberSelection(cmd, selection)
	var db *sql.DB
	if writable {
		db, err = store.OpenExistingWritable(cfg.DBPath)
	} else {
		db, err = store.OpenExisting(cfg.DBPath)
	}
	if err != nil {
		return nil, selection, nil, err
	}
	deps := &runtimeDeps{config: cfg, repo: store.NewRepository(db)}
	return deps, selection, cleanupRuntime(deps, db), nil
}

func prepareSelection(cmd *cobra.Command, dbFlag string, selectors []string, all, write bool) (resolvedSelection, config.Config, error) {
	selection := resolvedSelection{Requested: append([]string{}, selectors...)}
	if all && len(selectors) > 0 {
		return selection, config.Config{}, &commandError{Code: "scope_selector_invalid", Message: "--scope and --all-scopes are mutually exclusive"}
	}
	for _, selector := range selectors {
		if !validSelector(selector) {
			return selection, config.Config{}, &commandError{Code: "scope_selector_invalid", Message: fmt.Sprintf("invalid scope selector %q", selector)}
		}
	}
	cfg, err := config.Load(dbFlag)
	if err != nil {
		return selection, cfg, err
	}
	cwd, _ := cmd.Flags().GetString("cwd")
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return selection, cfg, err
		}
	}
	selection.CWD, _ = filepath.Abs(cwd)
	needsRepository := (!all && len(selectors) == 0) || slices.Contains(selectors, "repo")
	if needsRepository {
		observation, observeErr := repoctx.Observe(cmd.Context(), selection.CWD)
		switch {
		case observeErr == nil:
			selection.Observation = &observation
		case errors.Is(observeErr, repoctx.ErrOutsideRepository):
			selection.Resolution.Status = store.ResolutionOutside
			if write {
				return selection, cfg, writeScopeUnresolved()
			}
		case errors.Is(observeErr, repoctx.ErrAmbiguous):
			return selection, cfg, &commandError{Code: "repository_ambiguous", Message: "repository identity is ambiguous; configure thr.identityRemote"}
		default:
			return selection, cfg, &commandError{Code: "repository_unavailable", Message: "Git could not safely resolve repository context", Cause: observeErr}
		}
	}
	selection.Database, err = store.InspectOperationalDatabase(cfg.DBPath)
	if err != nil {
		return selection, cfg, err
	}
	if selection.Database.Status == store.DatabaseIncompatible {
		return selection, cfg, &commandError{Code: "database_version_incompatible", Message: "the selected database format is incompatible", Cause: store.ErrDatabaseIncompatible}
	}
	if selection.Observation != nil && selection.Observation.IdentityAmbiguous {
		if selection.Database.Status != store.DatabaseCompatible {
			return selection, cfg, &commandError{Code: "repository_ambiguous", Message: "repository identity is ambiguous; configure thr.identityRemote"}
		}
		db, openErr := store.OpenCompatibleReadOnly(cfg.DBPath)
		if openErr != nil {
			return selection, cfg, openErr
		}
		resolution, resolveErr := store.NewRepository(db).ResolveRepository(cmd.Context(), *selection.Observation)
		_ = db.Close()
		if resolveErr != nil || resolution.Scope == nil {
			return selection, cfg, &commandError{Code: "repository_ambiguous", Message: "repository identity is ambiguous; configure thr.identityRemote", Cause: resolveErr}
		}
		selection.Resolution = resolution
	}
	if selection.Database.Status == store.DatabaseMigrationRequired {
		migration, migrateErr := store.MigratePath(cmd.Context(), cfg.DBPath)
		if migrateErr != nil {
			return selection, cfg, &commandError{Code: "database_migration_required", Message: "the selected database requires migration", SuggestedCommand: "thr migrate", Cause: migrateErr}
		}
		selection.Migration = &migration
		selection.Database.Status = store.DatabaseCompatible
	}
	return selection, cfg, nil
}

func resolveReadScopes(cmd *cobra.Command, repo *store.Repository, selection *resolvedSelection, selectors []string, all bool) error {
	if all {
		infos, err := repo.ListScopes(cmd.Context())
		if err != nil {
			return err
		}
		selection.Mode = "all"
		for _, info := range infos {
			selection.Scopes = append(selection.Scopes, info.Scope)
		}
		return nil
	}
	if len(selectors) > 0 {
		selection.Mode = "explicit"
		for _, selector := range selectors {
			var scope domain.Scope
			if selector == "repo" {
				if selection.Observation == nil {
					return &commandError{Code: "repository_scope_unpersisted", Message: "the current repository has no persisted scope", SuggestedCommand: "thr scope create repo"}
				}
				resolution, err := repo.ResolveRepository(cmd.Context(), *selection.Observation)
				if err != nil {
					return mapRepositoryError(err)
				}
				selection.Resolution = resolution
				if resolution.Scope == nil {
					return &commandError{Code: "repository_scope_unpersisted", Message: "the current repository has no persisted scope", SuggestedCommand: "thr scope create repo"}
				}
				scope = *resolution.Scope
			} else {
				var err error
				scope, err = repo.GetScope(cmd.Context(), selector)
				if err != nil {
					return scopeNotFound(selector)
				}
			}
			if !slices.ContainsFunc(selection.Scopes, func(existing domain.Scope) bool { return existing.ID == scope.ID }) {
				selection.Scopes = append(selection.Scopes, scope)
			}
		}
		return nil
	}
	selection.Mode = "automatic_read"
	user, err := repo.GetScope(cmd.Context(), "user")
	if err != nil {
		return err
	}
	if selection.Observation != nil {
		resolution, err := repo.ResolveRepository(cmd.Context(), *selection.Observation)
		if err != nil {
			return mapRepositoryError(err)
		}
		selection.Resolution = resolution
		if resolution.Scope != nil {
			selection.Scopes = append(selection.Scopes, *resolution.Scope)
		}
	}
	selection.Scopes = append(selection.Scopes, user)
	return nil
}

func resolveMissingRead(selection *resolvedSelection, selectors []string, all bool) error {
	if all {
		selection.Mode, selection.Scopes = "all", []domain.Scope{}
		return nil
	}
	user := domain.Scope{ID: "user", Kind: domain.ScopeKindUser, Label: "user"}
	if len(selectors) == 0 {
		selection.Mode, selection.Scopes = "automatic_read", []domain.Scope{user}
		return nil
	}
	selection.Mode = "explicit"
	for _, selector := range selectors {
		if selector != "user" {
			if selector == "repo" {
				return &commandError{Code: "repository_scope_unpersisted", Message: "the current repository has no persisted scope", SuggestedCommand: "thr add <text>"}
			}
			return scopeNotFound(selector)
		}
		selection.Scopes = []domain.Scope{user}
	}
	return nil
}

func validSelector(value string) bool {
	return value == "user" || value == "repo" || strings.HasPrefix(value, "repo:") && len(value) > len("repo:")
}

func writeScopeUnresolved() error {
	return &commandError{Code: "write_scope_unresolved", Message: "cannot determine a default write scope outside a repository; use --scope user", SuggestedCommand: "thr add --scope user <text>"}
}

func scopeNotFound(selector string) error {
	return &commandError{Code: "scope_not_found", Message: fmt.Sprintf("scope %s not found", selector), Cause: store.ErrScopeNotFound}
}

func mapRepositoryError(err error) error {
	if errors.Is(err, store.ErrRepositoryAmbiguous) {
		return &commandError{Code: "repository_ambiguous", Message: "more than one scope matches this repository", Cause: err}
	}
	return err
}
