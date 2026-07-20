package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/repoctx"
)

var (
	ErrRepositoryAmbiguous        = errors.New("repository identity ambiguous")
	ErrRepositoryScopeUnpersisted = errors.New("repository scope is not persisted")
	ErrBindingConflict            = errors.New("repository checkout already bound")
	ErrConfirmationRequired       = errors.New("confirmation required")
)

type ResolutionStatus string

const (
	ResolutionBound       ResolutionStatus = "bound"
	ResolutionMatched     ResolutionStatus = "matched_unbound"
	ResolutionProspective ResolutionStatus = "prospective"
	ResolutionOutside     ResolutionStatus = "outside_repository"
	ResolutionAmbiguous   ResolutionStatus = "ambiguous"
	ResolutionUnavailable ResolutionStatus = "unavailable"
)

type RepositoryResolution struct {
	Status ResolutionStatus
	Scope  *domain.Scope
	Drift  bool
}

type ScopeInfo struct {
	domain.Scope
	MemoryCount int64
	Latest      *time.Time
	Bindings    []RepositoryBinding
	Aliases     []string
}

type RepositoryBinding struct {
	ID              int64
	ScopeID         string
	CommonDir       string
	WorktreeRoot    string
	RemoteName      string
	RemoteURL       string
	CanonicalRemote string
	Active          bool
}

func (r *Repository) GetScope(ctx context.Context, id string) (domain.Scope, error) {
	var scope domain.Scope
	err := r.db.QueryRowContext(ctx, `SELECT id, kind, label FROM scopes WHERE id = ?`, id).Scan(&scope.ID, &scope.Kind, &scope.Label)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Scope{}, ErrScopeNotFound
	}
	if err != nil {
		return domain.Scope{}, fmt.Errorf("fetch scope %s: %w", id, err)
	}
	return scope, nil
}

func (r *Repository) ListScopes(ctx context.Context) ([]ScopeInfo, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.kind, s.label, COUNT(m.id), MAX(m.updated_at)
		FROM scopes s LEFT JOIN scoped_memories m ON m.scope_id = s.id
		GROUP BY s.id ORDER BY CASE s.kind WHEN 'repo' THEN 0 ELSE 1 END, s.label, s.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list scopes: %w", err)
	}
	defer rows.Close()
	result := make([]ScopeInfo, 0)
	for rows.Next() {
		var item ScopeInfo
		var latest sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Kind, &item.Label, &item.MemoryCount, &latest); err != nil {
			return nil, fmt.Errorf("scan scope: %w", err)
		}
		if latest.Valid {
			value := time.UnixMilli(latest.Int64).UTC()
			item.Latest = &value
		}
		item.Bindings, err = r.listBindings(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		item.Aliases, err = r.listAliases(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) ResolveRepository(ctx context.Context, observation repoctx.Observation) (RepositoryResolution, error) {
	resolution := RepositoryResolution{}
	row := r.db.QueryRowContext(ctx, `
		SELECT s.id, s.kind, s.label, b.canonical_remote
		FROM repository_bindings b JOIN scopes s ON s.id = b.scope_id
		WHERE b.common_dir = ? AND b.active = 1
	`, observation.CommonDir)
	var scope domain.Scope
	var storedRemote string
	if err := row.Scan(&scope.ID, &scope.Kind, &scope.Label, &storedRemote); err == nil {
		resolution.Status, resolution.Scope = ResolutionBound, &scope
		resolution.Drift = storedRemote != observation.CanonicalRemote
		return resolution, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return resolution, fmt.Errorf("resolve repository binding: %w", err)
	}
	if observation.IdentityAmbiguous {
		resolution.Status = ResolutionAmbiguous
		return resolution, ErrRepositoryAmbiguous
	}
	return r.resolveRemote(ctx, observation, resolution)
}

func (r *Repository) resolveRemote(ctx context.Context, observation repoctx.Observation, resolution RepositoryResolution) (RepositoryResolution, error) {
	if observation.CanonicalRemote == "" {
		resolution.Status = ResolutionProspective
		return resolution, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT s.id, s.kind, s.label
		FROM repository_remote_aliases a JOIN scopes s ON s.id = a.scope_id
		WHERE a.canonical_remote = ? AND a.active = 1
	`, observation.CanonicalRemote)
	if err != nil {
		return resolution, fmt.Errorf("match repository remote: %w", err)
	}
	defer rows.Close()
	matches := make([]domain.Scope, 0, 2)
	for rows.Next() {
		var scope domain.Scope
		if err := rows.Scan(&scope.ID, &scope.Kind, &scope.Label); err != nil {
			return resolution, err
		}
		matches = append(matches, scope)
	}
	switch len(matches) {
	case 0:
		resolution.Status = ResolutionProspective
	case 1:
		resolution.Status, resolution.Scope = ResolutionMatched, &matches[0]
	default:
		resolution.Status = ResolutionAmbiguous
		return resolution, ErrRepositoryAmbiguous
	}
	return resolution, nil
}

func (r *Repository) EnsureRepositoryScope(ctx context.Context, observation repoctx.Observation) (domain.Scope, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Scope{}, fmt.Errorf("begin repository registration: %w", err)
	}
	defer rollback(tx)
	scope, err := ensureRepositoryScopeTx(ctx, tx, observation)
	if err != nil {
		return domain.Scope{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Scope{}, fmt.Errorf("commit repository registration for %s: %w", scope.ID, err)
	}
	return scope, nil
}

func ensureRepositoryScopeTx(ctx context.Context, tx *sql.Tx, observation repoctx.Observation) (domain.Scope, error) {
	if scope, found, err := boundScopeTx(ctx, tx, observation.CommonDir); err != nil {
		return domain.Scope{}, err
	} else if found {
		return scope, nil
	}

	var scope domain.Scope
	if observation.CanonicalRemote != "" {
		matches, err := matchingScopesTx(ctx, tx, observation.CanonicalRemote)
		if err != nil {
			return domain.Scope{}, err
		}
		if len(matches) > 1 {
			return domain.Scope{}, ErrRepositoryAmbiguous
		}
		if len(matches) == 1 {
			scope = matches[0]
		}
	}
	if scope.ID == "" {
		scope = domain.Scope{ID: "repo:" + rand.Text()[:26], Kind: domain.ScopeKindRepo, Label: observation.Label}
		now := time.Now().UTC().UnixMilli()
		if _, err := tx.ExecContext(ctx, `INSERT INTO scopes(id, kind, label, created_at, updated_at) VALUES (?, 'repo', ?, ?, ?)`, scope.ID, scope.Label, now, now); err != nil {
			return domain.Scope{}, fmt.Errorf("create repository scope: %w", err)
		}
	}
	if err := insertBindingTx(ctx, tx, scope.ID, observation); err != nil {
		return domain.Scope{}, err
	}
	return scope, nil
}

func (r *Repository) BindRepository(ctx context.Context, scopeID string, observation repoctx.Observation) (RepositoryBinding, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RepositoryBinding{}, err
	}
	defer rollback(tx)
	if _, err := getRepoScopeTx(ctx, tx, scopeID); err != nil {
		return RepositoryBinding{}, err
	}
	if _, found, err := boundScopeTx(ctx, tx, observation.CommonDir); err != nil {
		return RepositoryBinding{}, err
	} else if found {
		return RepositoryBinding{}, ErrBindingConflict
	}
	if err := insertBindingTx(ctx, tx, scopeID, observation); err != nil {
		return RepositoryBinding{}, err
	}
	binding, err := bindingTx(ctx, tx, observation.CommonDir)
	if err != nil {
		return RepositoryBinding{}, err
	}
	return binding, tx.Commit()
}

func (r *Repository) RebindRepository(ctx context.Context, scopeID string, observation repoctx.Observation) (RepositoryBinding, RepositoryBinding, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RepositoryBinding{}, RepositoryBinding{}, err
	}
	defer rollback(tx)
	if _, err := getRepoScopeTx(ctx, tx, scopeID); err != nil {
		return RepositoryBinding{}, RepositoryBinding{}, err
	}
	old, err := bindingTx(ctx, tx, observation.CommonDir)
	if err != nil {
		return RepositoryBinding{}, RepositoryBinding{}, err
	}
	now := time.Now().UTC().UnixMilli()
	if _, err := tx.ExecContext(ctx, `UPDATE repository_bindings SET scope_id = ?, updated_at = ? WHERE id = ?`, scopeID, now, old.ID); err != nil {
		return RepositoryBinding{}, RepositoryBinding{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE repository_remote_aliases SET scope_id = ? WHERE binding_id = ? AND active = 1`, scopeID, old.ID); err != nil {
		return RepositoryBinding{}, RepositoryBinding{}, err
	}
	if err := recordBindingHistory(ctx, tx, old, "rebind_from"); err != nil {
		return RepositoryBinding{}, RepositoryBinding{}, err
	}
	updated, err := bindingTx(ctx, tx, observation.CommonDir)
	if err != nil {
		return RepositoryBinding{}, RepositoryBinding{}, err
	}
	if err := recordBindingHistory(ctx, tx, updated, "rebind_to"); err != nil {
		return RepositoryBinding{}, RepositoryBinding{}, err
	}
	return old, updated, tx.Commit()
}

func (r *Repository) UnbindRepository(ctx context.Context, observation repoctx.Observation, confirmOrphan bool) (RepositoryBinding, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RepositoryBinding{}, err
	}
	defer rollback(tx)
	binding, err := bindingTx(ctx, tx, observation.CommonDir)
	if err != nil {
		return RepositoryBinding{}, err
	}
	var others int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM repository_bindings WHERE scope_id = ? AND active = 1 AND id != ?`, binding.ScopeID, binding.ID).Scan(&others); err != nil {
		return RepositoryBinding{}, err
	}
	if others == 0 && binding.CanonicalRemote == "" && !confirmOrphan {
		return RepositoryBinding{}, ErrConfirmationRequired
	}
	if _, err := tx.ExecContext(ctx, `UPDATE repository_bindings SET active = 0, updated_at = ? WHERE id = ?`, time.Now().UTC().UnixMilli(), binding.ID); err != nil {
		return RepositoryBinding{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE repository_remote_aliases SET active = 0 WHERE binding_id = ?`, binding.ID); err != nil {
		return RepositoryBinding{}, err
	}
	if err := recordBindingHistory(ctx, tx, binding, "unbind"); err != nil {
		return RepositoryBinding{}, err
	}
	return binding, tx.Commit()
}

func (r *Repository) RenameScope(ctx context.Context, scopeID, label string) (domain.Scope, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE scopes SET label = ?, updated_at = ? WHERE id = ? AND kind = 'repo'`, label, time.Now().UTC().UnixMilli(), scopeID)
	if err != nil {
		return domain.Scope{}, err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return domain.Scope{}, ErrScopeNotFound
	}
	return r.GetScope(ctx, scopeID)
}

func (r *Repository) SplitRepository(ctx context.Context, observation repoctx.Observation) (domain.Scope, RepositoryBinding, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Scope{}, RepositoryBinding{}, err
	}
	defer rollback(tx)
	binding, err := bindingTx(ctx, tx, observation.CommonDir)
	bound := err == nil
	if err != nil && !errors.Is(err, ErrRepositoryScopeUnpersisted) {
		return domain.Scope{}, RepositoryBinding{}, err
	}
	scope := domain.Scope{ID: "repo:" + rand.Text()[:26], Kind: domain.ScopeKindRepo, Label: observation.Label}
	now := time.Now().UTC().UnixMilli()
	if _, err := tx.ExecContext(ctx, `INSERT INTO scopes(id, kind, label, created_at, updated_at) VALUES (?, 'repo', ?, ?, ?)`, scope.ID, scope.Label, now, now); err != nil {
		return domain.Scope{}, RepositoryBinding{}, err
	}
	if !bound {
		if observation.CanonicalRemote != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE repository_remote_aliases SET intentionally_non_unique = 1 WHERE canonical_remote = ? AND active = 1`, observation.CanonicalRemote); err != nil {
				return domain.Scope{}, RepositoryBinding{}, err
			}
		}
		if err := insertBindingTx(ctx, tx, scope.ID, observation); err != nil {
			return domain.Scope{}, RepositoryBinding{}, err
		}
		binding, err = bindingTx(ctx, tx, observation.CommonDir)
		if err != nil {
			return domain.Scope{}, RepositoryBinding{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE repository_remote_aliases SET intentionally_non_unique = 1 WHERE binding_id = ?`, binding.ID); err != nil {
			return domain.Scope{}, RepositoryBinding{}, err
		}
		return scope, binding, tx.Commit()
	}
	if err := recordBindingHistory(ctx, tx, binding, "split_from"); err != nil {
		return domain.Scope{}, RepositoryBinding{}, err
	}
	if binding.CanonicalRemote != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE repository_remote_aliases SET binding_id = NULL, intentionally_non_unique = 1 WHERE binding_id = ? AND active = 1`, binding.ID); err != nil {
			return domain.Scope{}, RepositoryBinding{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO repository_remote_aliases(scope_id, binding_id, canonical_remote, normalization_version, active, intentionally_non_unique, observed_at) VALUES (?, ?, ?, ?, 1, 1, ?)`, scope.ID, binding.ID, binding.CanonicalRemote, observation.CanonicalizationVersion, now); err != nil {
			return domain.Scope{}, RepositoryBinding{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE repository_bindings SET scope_id = ?, updated_at = ? WHERE id = ?`, scope.ID, now, binding.ID); err != nil {
		return domain.Scope{}, RepositoryBinding{}, err
	}
	binding.ScopeID = scope.ID
	if err := recordBindingHistory(ctx, tx, binding, "split_to"); err != nil {
		return domain.Scope{}, RepositoryBinding{}, err
	}
	return scope, binding, tx.Commit()
}

func (r *Repository) listBindings(ctx context.Context, scopeID string) ([]RepositoryBinding, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, scope_id, common_dir, worktree_root, remote_name, remote_url, canonical_remote, active FROM repository_bindings WHERE scope_id = ? ORDER BY active DESC, id`, scopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bindings := make([]RepositoryBinding, 0)
	for rows.Next() {
		var binding RepositoryBinding
		if err := rows.Scan(&binding.ID, &binding.ScopeID, &binding.CommonDir, &binding.WorktreeRoot, &binding.RemoteName, &binding.RemoteURL, &binding.CanonicalRemote, &binding.Active); err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

func (r *Repository) listAliases(ctx context.Context, scopeID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT canonical_remote FROM repository_remote_aliases WHERE scope_id = ? AND active = 1 ORDER BY canonical_remote`, scopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	aliases := make([]string, 0)
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, err
		}
		aliases = append(aliases, alias)
	}
	return aliases, rows.Err()
}

func insertBindingTx(ctx context.Context, tx *sql.Tx, scopeID string, observation repoctx.Observation) error {
	now := time.Now().UTC().UnixMilli()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO repository_bindings(scope_id, common_dir, worktree_root, remote_name, remote_url, canonical_remote, normalization_version, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
	`, scopeID, observation.CommonDir, observation.WorktreeRoot, observation.RemoteName, observation.RemoteURL, observation.CanonicalRemote, observation.CanonicalizationVersion, now, now)
	if err != nil {
		return fmt.Errorf("bind repository checkout: %w", err)
	}
	id, _ := res.LastInsertId()
	if observation.CanonicalRemote != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO repository_remote_aliases(scope_id, binding_id, canonical_remote, normalization_version, active, observed_at) VALUES (?, ?, ?, ?, 1, ?)`, scopeID, id, observation.CanonicalRemote, observation.CanonicalizationVersion, now); err != nil {
			return fmt.Errorf("record repository remote alias: %w", err)
		}
	}
	return recordBindingHistory(ctx, tx, RepositoryBinding{ID: id, ScopeID: scopeID, CommonDir: observation.CommonDir, CanonicalRemote: observation.CanonicalRemote, Active: true}, "bind")
}

func boundScopeTx(ctx context.Context, tx *sql.Tx, commonDir string) (domain.Scope, bool, error) {
	var scope domain.Scope
	err := tx.QueryRowContext(ctx, `SELECT s.id, s.kind, s.label FROM repository_bindings b JOIN scopes s ON s.id = b.scope_id WHERE b.common_dir = ? AND b.active = 1`, commonDir).Scan(&scope.ID, &scope.Kind, &scope.Label)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Scope{}, false, nil
	}
	return scope, err == nil, err
}

func matchingScopesTx(ctx context.Context, tx *sql.Tx, remote string) ([]domain.Scope, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT s.id, s.kind, s.label FROM repository_remote_aliases a JOIN scopes s ON s.id = a.scope_id WHERE a.canonical_remote = ? AND a.active = 1`, remote)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scopes []domain.Scope
	for rows.Next() {
		var scope domain.Scope
		if err := rows.Scan(&scope.ID, &scope.Kind, &scope.Label); err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, rows.Err()
}

func getRepoScopeTx(ctx context.Context, tx *sql.Tx, id string) (domain.Scope, error) {
	scope, err := getScopeTx(ctx, tx, id)
	if err != nil {
		return domain.Scope{}, err
	}
	if scope.Kind != domain.ScopeKindRepo {
		return domain.Scope{}, ErrScopeNotFound
	}
	return scope, nil
}

func bindingTx(ctx context.Context, tx *sql.Tx, commonDir string) (RepositoryBinding, error) {
	var binding RepositoryBinding
	err := tx.QueryRowContext(ctx, `SELECT id, scope_id, common_dir, worktree_root, remote_name, remote_url, canonical_remote, active FROM repository_bindings WHERE common_dir = ? AND active = 1`, commonDir).Scan(
		&binding.ID, &binding.ScopeID, &binding.CommonDir, &binding.WorktreeRoot, &binding.RemoteName, &binding.RemoteURL, &binding.CanonicalRemote, &binding.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return RepositoryBinding{}, ErrRepositoryScopeUnpersisted
	}
	return binding, err
}

func recordBindingHistory(ctx context.Context, tx *sql.Tx, binding RepositoryBinding, action string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO repository_binding_history(binding_id, scope_id, action, common_dir, canonical_remote, observed_at) VALUES (?, ?, ?, ?, ?, ?)`,
		binding.ID, binding.ScopeID, action, binding.CommonDir, binding.CanonicalRemote, time.Now().UTC().UnixMilli())
	return err
}
