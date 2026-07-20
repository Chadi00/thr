package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/repoctx"
	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

var (
	ErrMemoryNotFound   = errors.New("memory not found")
	ErrScopeNotFound    = errors.New("scope not found")
	ErrScopeConflict    = errors.New("memory scope conflict")
	ErrRevisionConflict = errors.New("memory revision conflict")
)

const (
	DefaultListLimit           = 100
	DefaultKeywordLimit        = 10
	DefaultSemanticLimit       = 3
	DefaultSemanticMaxDistance = 0.80
	DefaultRecentWindow        = 2000
	DefaultRecallCandidateMin  = 64
	MaxListLimit               = 1000
	MaxSearchLimit             = 100
	MaxSemanticLimit           = 100
	MaxRecentWindow            = 5000
	MaxRecallCandidates        = 1000
)

const memoryColumns = `
	m.id, m.text, m.scope_assignment, m.created_at, m.updated_at,
	m.scope_updated_at, m.revision, s.id, s.kind, s.label`

type EmbeddingIdentity struct {
	ModelID        string
	ModelRevision  string
	ManifestSHA256 string
	Dimension      int
}

type IndexHealth struct {
	Memories          int64
	Indexed           int64
	Stale             int64
	MissingEmbeddings int64
}

type SemanticHit struct {
	Memory   domain.Memory
	Distance float64
}

type Preconditions struct {
	ScopeID  string
	Revision int64
}

type MoveResult struct {
	Memory         domain.Memory
	From           domain.Scope
	FromAssignment domain.ScopeAssignment
	To             domain.Scope
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) AddMemory(ctx context.Context, text string, embedding []float32, identity EmbeddingIdentity, scopeID string, assignment domain.ScopeAssignment) (domain.Memory, error) {
	now := time.UnixMilli(time.Now().UTC().UnixMilli()).UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Memory{}, fmt.Errorf("begin add memory transaction: %w", err)
	}
	defer rollback(tx)
	scope, err := getScopeTx(ctx, tx, scopeID)
	if err != nil {
		return domain.Memory{}, err
	}
	memory, err := addMemoryTx(ctx, tx, text, embedding, identity, scope, assignment, now)
	if err != nil {
		return domain.Memory{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Memory{}, fmt.Errorf("commit add memory transaction: %w", err)
	}
	return memory, nil
}

func (r *Repository) AddRepositoryMemory(ctx context.Context, text string, embedding []float32, identity EmbeddingIdentity, observation repoctx.Observation, assignment domain.ScopeAssignment) (domain.Memory, error) {
	now := time.UnixMilli(time.Now().UTC().UnixMilli()).UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Memory{}, fmt.Errorf("begin repository memory transaction: %w", err)
	}
	defer rollback(tx)
	scope, err := ensureRepositoryScopeTx(ctx, tx, observation)
	if err != nil {
		return domain.Memory{}, err
	}
	memory, err := addMemoryTx(ctx, tx, text, embedding, identity, scope, assignment, now)
	if err != nil {
		return domain.Memory{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Memory{}, fmt.Errorf("commit repository memory transaction: %w", err)
	}
	return memory, nil
}

func addMemoryTx(ctx context.Context, tx *sql.Tx, text string, embedding []float32, identity EmbeddingIdentity, scope domain.Scope, assignment domain.ScopeAssignment, now time.Time) (domain.Memory, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO scoped_memories(text, scope_id, scope_assignment, created_at, updated_at, scope_updated_at, revision)
		VALUES (?, ?, ?, ?, ?, ?, 1)
	`, text, scope.ID, assignment, now.UnixMilli(), now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return domain.Memory{}, fmt.Errorf("insert memory: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Memory{}, fmt.Errorf("get inserted memory id: %w", err)
	}
	if err := upsertEmbedding(ctx, tx, id, scope.ID, embedding, identity); err != nil {
		return domain.Memory{}, err
	}
	return domain.Memory{ID: id, Text: text, Scope: scope, ScopeAssignment: assignment, CreatedAt: now, UpdatedAt: now, ScopeUpdatedAt: now, Revision: 1}, nil
}

func (r *Repository) ListMemories(ctx context.Context, scopeIDs []string, limit int, legacyOnly bool) ([]domain.Memory, error) {
	limit = clampLimit(limit, DefaultListLimit, MaxListLimit)
	filter, args := scopeFilter("m.scope_id", scopeIDs)
	if legacyOnly {
		filter += ` AND m.scope_assignment = 'legacy_default'`
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `SELECT `+memoryColumns+`
		FROM scoped_memories m JOIN scopes s ON s.id = m.scope_id
		WHERE `+filter+` ORDER BY m.created_at DESC, m.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	return scanMemories(rows)
}

func (r *Repository) GetMemory(ctx context.Context, id int64) (domain.Memory, error) {
	return scanMemoryRow(r.db.QueryRowContext(ctx, `SELECT `+memoryColumns+`
		FROM scoped_memories m JOIN scopes s ON s.id = m.scope_id WHERE m.id = ?`, id), id)
}

func (r *Repository) EditMemory(ctx context.Context, id int64, text string, embedding []float32, identity EmbeddingIdentity, pre Preconditions) (domain.Memory, error) {
	now := time.UnixMilli(time.Now().UTC().UnixMilli()).UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Memory{}, fmt.Errorf("begin edit memory transaction: %w", err)
	}
	defer rollback(tx)
	current, err := getMemoryTx(ctx, tx, id)
	if err != nil {
		return domain.Memory{}, err
	}
	if err := checkPreconditions(current, pre); err != nil {
		return domain.Memory{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE scoped_memories SET text = ?, updated_at = ?, revision = revision + 1 WHERE id = ?`, text, now.UnixMilli(), id); err != nil {
		return domain.Memory{}, fmt.Errorf("update memory: %w", err)
	}
	if err := upsertEmbedding(ctx, tx, id, current.Scope.ID, embedding, identity); err != nil {
		return domain.Memory{}, err
	}
	memory, err := getMemoryTx(ctx, tx, id)
	if err != nil {
		return domain.Memory{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Memory{}, fmt.Errorf("commit edit memory transaction: %w", err)
	}
	return memory, nil
}

func (r *Repository) ForgetMemory(ctx context.Context, id int64, pre Preconditions) (domain.Memory, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Memory{}, fmt.Errorf("begin forget memory transaction: %w", err)
	}
	defer rollback(tx)
	memory, err := getMemoryTx(ctx, tx, id)
	if err != nil {
		return domain.Memory{}, err
	}
	if err := checkPreconditions(memory, pre); err != nil {
		return domain.Memory{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM scoped_memory_embeddings WHERE rowid = ?`, id); err != nil {
		return domain.Memory{}, fmt.Errorf("delete embedding row: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM scoped_memories WHERE id = ?`, id); err != nil {
		return domain.Memory{}, fmt.Errorf("delete memory: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.Memory{}, fmt.Errorf("commit forget memory transaction: %w", err)
	}
	return memory, nil
}

func (r *Repository) MoveMemory(ctx context.Context, id int64, destination string, pre Preconditions) (MoveResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return MoveResult{}, fmt.Errorf("begin move memory transaction: %w", err)
	}
	defer rollback(tx)
	memory, err := getMemoryTx(ctx, tx, id)
	if err != nil {
		return MoveResult{}, err
	}
	if err := checkPreconditions(memory, pre); err != nil {
		return MoveResult{}, err
	}
	target, err := getScopeTx(ctx, tx, destination)
	if err != nil {
		return MoveResult{}, err
	}
	result, err := moveMemoryTx(ctx, tx, memory, target)
	if err != nil {
		return MoveResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return MoveResult{}, fmt.Errorf("commit move memory transaction: %w", err)
	}
	return result, nil
}

func (r *Repository) MoveMemoryToRepository(ctx context.Context, id int64, observation repoctx.Observation, pre Preconditions) (MoveResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return MoveResult{}, fmt.Errorf("begin repository move transaction: %w", err)
	}
	defer rollback(tx)
	memory, err := getMemoryTx(ctx, tx, id)
	if err != nil {
		return MoveResult{}, err
	}
	if err := checkPreconditions(memory, pre); err != nil {
		return MoveResult{}, err
	}
	target, err := ensureRepositoryScopeTx(ctx, tx, observation)
	if err != nil {
		return MoveResult{}, err
	}
	result, err := moveMemoryTx(ctx, tx, memory, target)
	if err != nil {
		return MoveResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return MoveResult{}, fmt.Errorf("commit repository move transaction: %w", err)
	}
	return result, nil
}

func moveMemoryTx(ctx context.Context, tx *sql.Tx, memory domain.Memory, target domain.Scope) (MoveResult, error) {
	result := MoveResult{Memory: memory, From: memory.Scope, FromAssignment: memory.ScopeAssignment, To: target}
	if memory.Scope.ID == target.ID && memory.ScopeAssignment != domain.ScopeAssignmentLegacy {
		return result, nil
	}
	now := time.Now().UTC().UnixMilli()
	if memory.Scope.ID != target.ID {
		var vector []byte
		vectorErr := tx.QueryRowContext(ctx, `SELECT embedding FROM scoped_memory_embeddings WHERE rowid = ?`, memory.ID).Scan(&vector)
		if vectorErr != nil && !errors.Is(vectorErr, sql.ErrNoRows) {
			return MoveResult{}, fmt.Errorf("read embedding for move: %w", vectorErr)
		}
		if vectorErr == nil {
			if _, err := tx.ExecContext(ctx, `DELETE FROM scoped_memory_embeddings WHERE rowid = ?`, memory.ID); err != nil {
				return MoveResult{}, fmt.Errorf("remove embedding for move: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_memory_embeddings(rowid, scope_id, embedding) VALUES (?, ?, ?)`, memory.ID, target.ID, vector); err != nil {
				return MoveResult{}, fmt.Errorf("move embedding: %w", err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE scoped_memories SET scope_id = ?, scope_assignment = 'explicit', scope_updated_at = ?, revision = revision + 1 WHERE id = ?
	`, target.ID, now, memory.ID); err != nil {
		return MoveResult{}, fmt.Errorf("move memory: %w", err)
	}
	updated, err := getMemoryTx(ctx, tx, memory.ID)
	if err != nil {
		return MoveResult{}, err
	}
	result.Memory = updated
	return result, nil
}

func (r *Repository) SemanticSearch(ctx context.Context, embedding []float32, scopeIDs []string, limit int, identity EmbeddingIdentity, maxDistance float64) ([]SemanticHit, error) {
	limit = clampLimit(limit, DefaultSemanticLimit, MaxSemanticLimit)
	query, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query embedding: %w", err)
	}
	hits := make([]SemanticHit, 0, len(scopeIDs)*limit)
	seen := make(map[int64]struct{})
	// ponytail: one exact KNN per scope keeps pre-limit filtering correct; batch only if all-scope profiling needs it.
	for _, scopeID := range scopeIDs {
		rows, err := r.db.QueryContext(ctx, `SELECT `+memoryColumns+`, e.distance
			FROM scoped_memory_embeddings e
			JOIN scoped_memory_embedding_metadata md ON md.memory_id = e.rowid
			JOIN scoped_memories m ON m.id = e.rowid AND m.scope_id = e.scope_id
			JOIN scopes s ON s.id = m.scope_id
			WHERE e.embedding MATCH ? AND e.k = ? AND e.scope_id = ?
			  AND md.model_id = ? AND md.model_revision = ? AND md.manifest_sha256 = ? AND md.dimension = ?
			ORDER BY e.distance`, query, limit, scopeID, identity.ModelID, identity.ModelRevision, identity.ManifestSHA256, identity.Dimension)
		if err != nil {
			return nil, fmt.Errorf("semantic search query: %w", err)
		}
		for rows.Next() {
			memory, distance, err := scanSemanticRow(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			if maxDistance > 0 && distance > maxDistance {
				continue
			}
			if _, exists := seen[memory.ID]; exists {
				continue
			}
			seen[memory.ID] = struct{}{}
			hits = append(hits, SemanticHit{Memory: memory, Distance: distance})
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		ar, br := math.Round(a.Distance*1e6), math.Round(b.Distance*1e6)
		if ar != br {
			return ar < br
		}
		if a.Memory.Scope.Kind != b.Memory.Scope.Kind {
			return a.Memory.Scope.Kind == domain.ScopeKindRepo
		}
		if a.Distance != b.Distance {
			return a.Distance < b.Distance
		}
		if !a.Memory.UpdatedAt.Equal(b.Memory.UpdatedAt) {
			return a.Memory.UpdatedAt.After(b.Memory.UpdatedAt)
		}
		return a.Memory.ID < b.Memory.ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func (r *Repository) KeywordSearch(ctx context.Context, query string, scopeIDs []string, limit int) ([]domain.Memory, error) {
	limit = clampLimit(limit, DefaultKeywordLimit, MaxRecallCandidates)
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return []domain.Memory{}, nil
	}
	filter, args := scopeFilter("m.scope_id", scopeIDs)
	args = append([]any{ftsQuery}, args...)
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `SELECT `+memoryColumns+`
		FROM scoped_memory_fts f
		JOIN scoped_memories m ON m.id = f.rowid
		JOIN scopes s ON s.id = m.scope_id
		WHERE scoped_memory_fts MATCH ? AND `+filter+`
		ORDER BY bm25(scoped_memory_fts) LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("keyword search query: %w", err)
	}
	return scanMemories(rows)
}

func (r *Repository) loadRecentWindow(ctx context.Context, scopeIDs []string, recentWindow int) ([]domain.Memory, error) {
	recentWindow = clampLimit(recentWindow, DefaultRecentWindow, MaxRecentWindow)
	filter, args := scopeFilter("m.scope_id", scopeIDs)
	args = append(args, recentWindow)
	rows, err := r.db.QueryContext(ctx, `SELECT `+memoryColumns+`
		FROM scoped_memories m JOIN scopes s ON s.id = m.scope_id
		WHERE `+filter+` ORDER BY m.updated_at DESC, m.id ASC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("load recent memories: %w", err)
	}
	return scanMemories(rows)
}

func (r *Repository) RecallSearch(ctx context.Context, query string, scopeIDs []string, limit, recentWindow, candidateLimit int) ([]domain.Memory, error) {
	limit = clampLimit(limit, DefaultKeywordLimit, MaxSearchLimit)
	recentWindow = clampLimit(recentWindow, DefaultRecentWindow, MaxRecentWindow)
	if candidateLimit <= 0 {
		candidateLimit = max(limit*8, DefaultRecallCandidateMin)
	}
	candidateLimit = min(candidateLimit, MaxRecallCandidates)
	candidates := make([]domain.Memory, 0, candidateLimit)
	seen := make(map[int64]struct{}, candidateLimit)
	ftsHits, err := r.KeywordSearch(ctx, query, scopeIDs, candidateLimit)
	if err != nil {
		return nil, err
	}
	for _, memory := range ftsHits {
		if len(candidates) >= candidateLimit {
			break
		}
		seen[memory.ID] = struct{}{}
		candidates = append(candidates, memory)
	}
	if len(candidates) < candidateLimit {
		recent, err := r.loadRecentWindow(ctx, scopeIDs, recentWindow)
		if err != nil {
			return nil, err
		}
		needle := strings.ToLower(strings.TrimSpace(query))
		for _, memory := range recent {
			if len(candidates) >= candidateLimit {
				break
			}
			if _, exists := seen[memory.ID]; !exists && needle != "" && strings.Contains(strings.ToLower(memory.Text), needle) {
				seen[memory.ID] = struct{}{}
				candidates = append(candidates, memory)
			}
		}
		if len(candidates) < candidateLimit {
			type extraHit struct {
				memory domain.Memory
				score  int
			}
			extras := make([]extraHit, 0)
			for _, memory := range recent {
				if _, exists := seen[memory.ID]; exists {
					continue
				}
				if score := fuzzyScore(query, memory.Text); score >= 0 {
					extras = append(extras, extraHit{memory, score})
				}
			}
			sort.Slice(extras, func(i, j int) bool {
				if extras[i].score != extras[j].score {
					return extras[i].score > extras[j].score
				}
				if !extras[i].memory.UpdatedAt.Equal(extras[j].memory.UpdatedAt) {
					return extras[i].memory.UpdatedAt.After(extras[j].memory.UpdatedAt)
				}
				return extras[i].memory.ID < extras[j].memory.ID
			})
			for _, extra := range extras {
				if len(candidates) >= candidateLimit {
					break
				}
				seen[extra.memory.ID] = struct{}{}
				candidates = append(candidates, extra.memory)
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := fuzzyScore(query, candidates[i].Text), fuzzyScore(query, candidates[j].Text)
		if a != b {
			return a > b
		}
		if !candidates[i].UpdatedAt.Equal(candidates[j].UpdatedAt) {
			return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt)
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

func (r *Repository) CountMemories(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scoped_memories`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count memories: %w", err)
	}
	return count, nil
}

func (r *Repository) ListMemoriesNeedingIndex(ctx context.Context, scopeIDs []string, identity EmbeddingIdentity) ([]domain.Memory, error) {
	filter, scopeArgs := scopeFilter("m.scope_id", scopeIDs)
	args := append(scopeArgs, identity.ModelID, identity.ModelRevision, identity.ManifestSHA256, identity.Dimension)
	rows, err := r.db.QueryContext(ctx, `SELECT `+memoryColumns+`
		FROM scoped_memories m
		JOIN scopes s ON s.id = m.scope_id
		LEFT JOIN scoped_memory_embeddings e ON e.rowid = m.id
		LEFT JOIN scoped_memory_embedding_metadata md ON md.memory_id = m.id
		WHERE `+filter+` AND (e.rowid IS NULL OR md.memory_id IS NULL OR e.scope_id != m.scope_id
			OR md.model_id != ? OR md.model_revision != ? OR md.manifest_sha256 != ? OR md.dimension != ?)
		ORDER BY m.id ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list memories needing index: %w", err)
	}
	return scanMemories(rows)
}

func (r *Repository) UpsertMemoryEmbedding(ctx context.Context, id int64, embedding []float32, identity EmbeddingIdentity, expectedRevision int64, expectedScope string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin embedding update transaction: %w", err)
	}
	defer rollback(tx)
	memory, err := getMemoryTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if expectedRevision > 0 && memory.Revision != expectedRevision {
		return ErrRevisionConflict
	}
	if expectedScope != "" && memory.Scope.ID != expectedScope {
		return ErrScopeConflict
	}
	if err := upsertEmbedding(ctx, tx, id, memory.Scope.ID, embedding, identity); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit embedding update transaction: %w", err)
	}
	return nil
}

func (r *Repository) IndexHealth(ctx context.Context, scopeIDs []string, identity EmbeddingIdentity) (IndexHealth, error) {
	filter, scopeArgs := scopeFilter("m.scope_id", scopeIDs)
	args := []any{identity.ModelID, identity.ModelRevision, identity.ManifestSHA256, identity.Dimension,
		identity.ModelID, identity.ModelRevision, identity.ManifestSHA256, identity.Dimension}
	args = append(args, scopeArgs...)
	var health IndexHealth
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN e.rowid IS NOT NULL AND md.memory_id IS NOT NULL AND e.scope_id = m.scope_id
				AND md.model_id = ? AND md.model_revision = ? AND md.manifest_sha256 = ? AND md.dimension = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN e.rowid IS NOT NULL AND md.memory_id IS NOT NULL AND (e.scope_id != m.scope_id
				OR md.model_id != ? OR md.model_revision != ? OR md.manifest_sha256 != ? OR md.dimension != ?) THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN e.rowid IS NULL OR md.memory_id IS NULL THEN 1 ELSE 0 END), 0)
		FROM scoped_memories m
		LEFT JOIN scoped_memory_embeddings e ON e.rowid = m.id
		LEFT JOIN scoped_memory_embedding_metadata md ON md.memory_id = m.id
		WHERE `+filter,
		args...,
	).Scan(&health.Memories, &health.Indexed, &health.Stale, &health.MissingEmbeddings)
	if err != nil {
		return IndexHealth{}, fmt.Errorf("read index health: %w", err)
	}
	return health, nil
}

func upsertEmbedding(ctx context.Context, tx *sql.Tx, id int64, scopeID string, embedding []float32, identity EmbeddingIdentity) error {
	blob, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return fmt.Errorf("serialize embedding: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM scoped_memory_embeddings WHERE rowid = ?`, id); err != nil {
		return fmt.Errorf("delete previous embedding: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO scoped_memory_embeddings(rowid, scope_id, embedding) VALUES (?, ?, ?)`, id, scopeID, blob); err != nil {
		return fmt.Errorf("insert embedding: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO scoped_memory_embedding_metadata(memory_id, model_id, model_revision, manifest_sha256, dimension, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(memory_id) DO UPDATE SET model_id=excluded.model_id, model_revision=excluded.model_revision,
			manifest_sha256=excluded.manifest_sha256, dimension=excluded.dimension, indexed_at=excluded.indexed_at
	`, id, identity.ModelID, identity.ModelRevision, identity.ManifestSHA256, identity.Dimension, time.Now().UTC().UnixMilli()); err != nil {
		return fmt.Errorf("record embedding metadata: %w", err)
	}
	return nil
}

func getMemoryTx(ctx context.Context, tx *sql.Tx, id int64) (domain.Memory, error) {
	return scanMemoryRow(tx.QueryRowContext(ctx, `SELECT `+memoryColumns+`
		FROM scoped_memories m JOIN scopes s ON s.id = m.scope_id WHERE m.id = ?`, id), id)
}

func getScopeTx(ctx context.Context, tx *sql.Tx, id string) (domain.Scope, error) {
	var scope domain.Scope
	if err := tx.QueryRowContext(ctx, `SELECT id, kind, label FROM scopes WHERE id = ?`, id).Scan(&scope.ID, &scope.Kind, &scope.Label); errors.Is(err, sql.ErrNoRows) {
		return domain.Scope{}, ErrScopeNotFound
	} else if err != nil {
		return domain.Scope{}, fmt.Errorf("fetch scope %s: %w", id, err)
	}
	return scope, nil
}

type rowScanner interface{ Scan(...any) error }

func scanMemoryRow(row rowScanner, id int64) (domain.Memory, error) {
	var memory domain.Memory
	var created, updated, scopeUpdated int64
	err := row.Scan(&memory.ID, &memory.Text, &memory.ScopeAssignment, &created, &updated, &scopeUpdated, &memory.Revision, &memory.Scope.ID, &memory.Scope.Kind, &memory.Scope.Label)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Memory{}, ErrMemoryNotFound
	}
	if err != nil {
		return domain.Memory{}, fmt.Errorf("fetch memory %d: %w", id, err)
	}
	memory.CreatedAt = time.UnixMilli(created).UTC()
	memory.UpdatedAt = time.UnixMilli(updated).UTC()
	memory.ScopeUpdatedAt = time.UnixMilli(scopeUpdated).UTC()
	return memory, nil
}

func scanMemories(rows *sql.Rows) ([]domain.Memory, error) {
	defer rows.Close()
	memories := make([]domain.Memory, 0)
	for rows.Next() {
		memory, err := scanMemoryRow(rows, 0)
		if err != nil {
			return nil, err
		}
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memories: %w", err)
	}
	return memories, nil
}

func scanSemanticRow(rows *sql.Rows) (domain.Memory, float64, error) {
	var memory domain.Memory
	var created, updated, scopeUpdated int64
	var distance float64
	if err := rows.Scan(&memory.ID, &memory.Text, &memory.ScopeAssignment, &created, &updated, &scopeUpdated, &memory.Revision,
		&memory.Scope.ID, &memory.Scope.Kind, &memory.Scope.Label, &distance); err != nil {
		return domain.Memory{}, 0, fmt.Errorf("scan semantic hit: %w", err)
	}
	memory.CreatedAt = time.UnixMilli(created).UTC()
	memory.UpdatedAt = time.UnixMilli(updated).UTC()
	memory.ScopeUpdatedAt = time.UnixMilli(scopeUpdated).UTC()
	return memory, distance, nil
}

func checkPreconditions(memory domain.Memory, pre Preconditions) error {
	if pre.ScopeID != "" && pre.ScopeID != memory.Scope.ID {
		return ErrScopeConflict
	}
	if pre.Revision > 0 && pre.Revision != memory.Revision {
		return ErrRevisionConflict
	}
	return nil
}

func scopeFilter(column string, ids []string) (string, []any) {
	if len(ids) == 0 {
		return "1 = 0", nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i], args[i] = "?", id
	}
	return column + " IN (" + strings.Join(placeholders, ",") + ")", args
}

func rollback(tx *sql.Tx) { _ = tx.Rollback() }

func buildFTSQuery(query string) string {
	tokens := tokenizeFTSQuery(query)
	clauses := make([]string, 0, len(tokens))
	for _, token := range tokens {
		clauses = append(clauses, buildFTSTokenClause(token))
	}
	return strings.Join(clauses, " OR ")
}

func tokenizeFTSQuery(query string) []string {
	return strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_'
	})
}

func buildFTSTokenClause(token string) string {
	quoted := quoteFTSTerm(token)
	if isSafeFTSPrefixToken(token) {
		return fmt.Sprintf("(%s OR %s*)", quoted, token)
	}
	return quoted
}

func quoteFTSTerm(token string) string {
	return fmt.Sprintf("\"%s\"", strings.ReplaceAll(token, "\"", "\"\""))
}

func isSafeFTSPrefixToken(token string) bool {
	switch token {
	case "and", "or", "not", "near":
		return false
	}
	for _, r := range token {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func fuzzyScore(query, text string) int {
	needle := strings.ToLower(strings.TrimSpace(query))
	haystack := strings.ToLower(text)
	if needle == "" || haystack == "" {
		return -1
	}
	if idx := strings.Index(haystack, needle); idx >= 0 {
		return 100000 - len([]rune(haystack[:idx]))
	}
	needleRunes, haystackRunes := compactSpaceRunes(needle), compactSpaceRunes(haystack)
	if len(needleRunes) == 0 || len(haystackRunes) == 0 {
		return -1
	}
	position, gaps := 0, 0
	for _, want := range needleRunes {
		next := indexRune(haystackRunes[position:], want)
		if next < 0 {
			return -1
		}
		gaps += next
		position += next + 1
	}
	return 5000 - gaps
}

func compactSpaceRunes(value string) []rune {
	out := make([]rune, 0, len(value))
	for _, r := range value {
		if !unicode.IsSpace(r) {
			out = append(out, r)
		}
	}
	return out
}

func indexRune(values []rune, want rune) int {
	for i, got := range values {
		if got == want {
			return i
		}
	}
	return -1
}

func clampLimit(value, defaultValue, maxValue int) int {
	if value <= 0 {
		return defaultValue
	}
	return min(value, maxValue)
}
