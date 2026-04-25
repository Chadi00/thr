package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Chadi00/thr/internal/domain"
	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

var ErrMemoryNotFound = errors.New("memory not found")

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

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) AddMemory(ctx context.Context, text string, embedding []float32, identity EmbeddingIdentity) (domain.Memory, error) {
	now := time.Now().UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Memory{}, fmt.Errorf("begin add memory transaction: %w", err)
	}
	defer rollback(tx)

	res, err := tx.ExecContext(ctx, `
		INSERT INTO memories (text, created_at, updated_at)
		VALUES (?, ?, ?)
	`, text, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return domain.Memory{}, fmt.Errorf("insert memory: %w", err)
	}

	memoryID, err := res.LastInsertId()
	if err != nil {
		return domain.Memory{}, fmt.Errorf("get inserted memory id: %w", err)
	}

	if err := upsertEmbedding(ctx, tx, memoryID, embedding, identity); err != nil {
		return domain.Memory{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Memory{}, fmt.Errorf("commit add memory transaction: %w", err)
	}

	return domain.Memory{
		ID:        memoryID,
		Text:      text,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (r *Repository) ListMemories(ctx context.Context, limit int) ([]domain.Memory, error) {
	limit = clampLimit(limit, DefaultListLimit, MaxListLimit)

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, text, created_at, updated_at
		FROM memories
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	defer rows.Close()

	memories := make([]domain.Memory, 0)
	for rows.Next() {
		memory, err := scanMemory(rows)
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

func (r *Repository) GetMemory(ctx context.Context, id int64) (domain.Memory, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, text, created_at, updated_at
		FROM memories
		WHERE id = ?
	`, id)

	var (
		memory                         domain.Memory
		createdUnixMillis, updatedUnix int64
	)
	err := row.Scan(&memory.ID, &memory.Text, &createdUnixMillis, &updatedUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Memory{}, ErrMemoryNotFound
	}
	if err != nil {
		return domain.Memory{}, fmt.Errorf("fetch memory %d: %w", id, err)
	}

	memory.CreatedAt = time.UnixMilli(createdUnixMillis).UTC()
	memory.UpdatedAt = time.UnixMilli(updatedUnix).UTC()
	return memory, nil
}

func (r *Repository) EditMemory(ctx context.Context, id int64, text string, embedding []float32, identity EmbeddingIdentity) (domain.Memory, error) {
	now := time.Now().UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Memory{}, fmt.Errorf("begin edit memory transaction: %w", err)
	}
	defer rollback(tx)

	res, err := tx.ExecContext(ctx, `
		UPDATE memories
		SET text = ?, updated_at = ?
		WHERE id = ?
	`, text, now.UnixMilli(), id)
	if err != nil {
		return domain.Memory{}, fmt.Errorf("update memory: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Memory{}, fmt.Errorf("read edit affected rows: %w", err)
	}
	if affected == 0 {
		return domain.Memory{}, ErrMemoryNotFound
	}

	if err := upsertEmbedding(ctx, tx, id, embedding, identity); err != nil {
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
func (r *Repository) ForgetMemory(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin forget memory transaction: %w", err)
	}
	defer rollback(tx)

	if _, err := getMemoryTx(ctx, tx, id); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_embeddings WHERE rowid = ?`, id); err != nil {
		return fmt.Errorf("delete embedding row: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_embedding_metadata WHERE memory_id = ?`, id); err != nil {
		return fmt.Errorf("delete embedding metadata: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit forget memory transaction: %w", err)
	}

	return nil
}

func (r *Repository) SemanticSearch(ctx context.Context, embedding []float32, limit int, identity EmbeddingIdentity, maxDistance float64) ([]SemanticHit, error) {
	limit = clampLimit(limit, DefaultSemanticLimit, MaxSemanticLimit)
	query, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query embedding: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.text, m.created_at, m.updated_at, e.distance
		FROM memory_embeddings e
		JOIN memory_embedding_metadata md ON md.memory_id = e.rowid
		JOIN memories m ON m.id = e.rowid
		WHERE e.embedding MATCH ? AND e.k = ?
		  AND md.model_id = ?
		  AND md.model_revision = ?
		  AND md.manifest_sha256 = ?
		  AND md.dimension = ?
		ORDER BY e.distance
	`, query, limit, identity.ModelID, identity.ModelRevision, identity.ManifestSHA256, identity.Dimension)
	if err != nil {
		return nil, fmt.Errorf("semantic search query: %w", err)
	}
	defer rows.Close()

	hits := make([]SemanticHit, 0)
	for rows.Next() {
		var (
			memory                         domain.Memory
			createdUnixMillis, updatedUnix int64
			distance                       float64
		)
		if err := rows.Scan(&memory.ID, &memory.Text, &createdUnixMillis, &updatedUnix, &distance); err != nil {
			return nil, fmt.Errorf("scan semantic hit: %w", err)
		}
		memory.CreatedAt = time.UnixMilli(createdUnixMillis).UTC()
		memory.UpdatedAt = time.UnixMilli(updatedUnix).UTC()
		if maxDistance > 0 && distance > maxDistance {
			continue
		}
		hits = append(hits, SemanticHit{Memory: memory, Distance: distance})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate semantic hits: %w", err)
	}

	return hits, nil
}

func (r *Repository) KeywordSearch(ctx context.Context, query string, limit int) ([]domain.Memory, error) {
	limit = clampLimit(limit, DefaultKeywordLimit, MaxRecallCandidates)
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return []domain.Memory{}, nil
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.text, m.created_at, m.updated_at
		FROM memory_fts f
		JOIN memories m ON m.id = f.rowid
		WHERE memory_fts MATCH ?
		ORDER BY bm25(memory_fts)
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("keyword search query: %w", err)
	}
	defer rows.Close()

	results := make([]domain.Memory, 0)
	for rows.Next() {
		memory, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, memory)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate keyword results: %w", err)
	}

	return results, nil
}

func (r *Repository) loadRecentWindow(ctx context.Context, recentWindow int) ([]domain.Memory, error) {
	recentWindow = clampLimit(recentWindow, DefaultRecentWindow, MaxRecentWindow)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, text, created_at, updated_at
		FROM memories
		ORDER BY updated_at DESC
		LIMIT ?
	`, recentWindow)
	if err != nil {
		return nil, fmt.Errorf("load recent memories: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Memory, 0, recentWindow)
	for rows.Next() {
		memory, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent memories: %w", err)
	}
	return out, nil
}

func (r *Repository) RecallSearch(ctx context.Context, query string, limit int, recentWindow int, candidateLimit int) ([]domain.Memory, error) {
	limit = clampLimit(limit, DefaultKeywordLimit, MaxSearchLimit)
	recentWindow = clampLimit(recentWindow, DefaultRecentWindow, MaxRecentWindow)
	if candidateLimit <= 0 {
		candidateLimit = max(limit*8, DefaultRecallCandidateMin)
	}
	candidateLimit = min(candidateLimit, MaxRecallCandidates)

	candidates := make([]domain.Memory, 0, candidateLimit)
	seen := make(map[int64]struct{}, candidateLimit)

	ftsHits, err := r.KeywordSearch(ctx, query, candidateLimit)
	if err != nil {
		return nil, err
	}
	for _, memory := range ftsHits {
		if len(candidates) >= candidateLimit {
			break
		}
		if _, exists := seen[memory.ID]; exists {
			continue
		}
		seen[memory.ID] = struct{}{}
		candidates = append(candidates, memory)
	}

	if len(candidates) < candidateLimit {
		recent, err := r.loadRecentWindow(ctx, recentWindow)
		if err != nil {
			return nil, err
		}
		need := candidateLimit - len(candidates)
		needle := strings.ToLower(strings.TrimSpace(query))
		if needle != "" {
			for _, memory := range recent {
				if need <= 0 {
					break
				}
				if _, exists := seen[memory.ID]; exists {
					continue
				}
				if strings.Contains(strings.ToLower(memory.Text), needle) {
					seen[memory.ID] = struct{}{}
					candidates = append(candidates, memory)
					need--
				}
			}
		}
		if len(candidates) < candidateLimit {
			need = candidateLimit - len(candidates)
			type extraHit struct {
				memory domain.Memory
				score  int
			}
			extras := make([]extraHit, 0)
			for _, memory := range recent {
				if _, exists := seen[memory.ID]; exists {
					continue
				}
				if s := fuzzyScore(query, memory.Text); s >= 0 {
					extras = append(extras, extraHit{memory: memory, score: s})
				}
			}
			sort.SliceStable(extras, func(i, j int) bool {
				if extras[i].score != extras[j].score {
					return extras[i].score > extras[j].score
				}
				return extras[i].memory.UpdatedAt.After(extras[j].memory.UpdatedAt)
			})
			for i := 0; i < len(extras) && need > 0; i++ {
				seen[extras[i].memory.ID] = struct{}{}
				candidates = append(candidates, extras[i].memory)
				need--
			}
		}
	}

	type scored struct {
		memory domain.Memory
		score  int
	}
	scoredCandidates := make([]scored, 0, len(candidates))
	for _, candidate := range candidates {
		scoredCandidates = append(scoredCandidates, scored{
			memory: candidate,
			score:  fuzzyScore(query, candidate.Text),
		})
	}
	sort.SliceStable(scoredCandidates, func(i, j int) bool {
		if scoredCandidates[i].score != scoredCandidates[j].score {
			return scoredCandidates[i].score > scoredCandidates[j].score
		}
		return scoredCandidates[i].memory.UpdatedAt.After(scoredCandidates[j].memory.UpdatedAt)
	})

	results := make([]domain.Memory, 0, min(limit, len(scoredCandidates)))
	for _, hit := range scoredCandidates {
		if len(results) >= limit {
			break
		}
		results = append(results, hit.memory)
	}
	return results, nil
}

func (r *Repository) CountMemories(ctx context.Context) (int64, error) {
	row := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories`)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count memories: %w", err)
	}
	return count, nil
}

func (r *Repository) ListMemoriesNeedingIndex(ctx context.Context, identity EmbeddingIdentity) ([]domain.Memory, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.text, m.created_at, m.updated_at
		FROM memories m
		LEFT JOIN memory_embeddings e ON e.rowid = m.id
		LEFT JOIN memory_embedding_metadata md ON md.memory_id = m.id
		WHERE e.rowid IS NULL
		   OR md.memory_id IS NULL
		   OR md.model_id != ?
		   OR md.model_revision != ?
		   OR md.manifest_sha256 != ?
		   OR md.dimension != ?
		ORDER BY m.id ASC
	`, identity.ModelID, identity.ModelRevision, identity.ManifestSHA256, identity.Dimension)
	if err != nil {
		return nil, fmt.Errorf("list memories needing index: %w", err)
	}
	defer rows.Close()

	memories := make([]domain.Memory, 0)
	for rows.Next() {
		memory, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate memories needing index: %w", err)
	}
	return memories, nil
}

func (r *Repository) UpsertMemoryEmbedding(ctx context.Context, id int64, embedding []float32, identity EmbeddingIdentity) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin embedding update transaction: %w", err)
	}
	defer rollback(tx)

	if _, err := getMemoryTx(ctx, tx, id); err != nil {
		return err
	}
	if err := upsertEmbedding(ctx, tx, id, embedding, identity); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit embedding update transaction: %w", err)
	}
	return nil
}

func (r *Repository) IndexHealth(ctx context.Context, identity EmbeddingIdentity) (IndexHealth, error) {
	var health IndexHealth
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories`).Scan(&health.Memories); err != nil {
		return IndexHealth{}, fmt.Errorf("count memories: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM memories m
		JOIN memory_embeddings e ON e.rowid = m.id
		JOIN memory_embedding_metadata md ON md.memory_id = m.id
		WHERE md.model_id = ?
		  AND md.model_revision = ?
		  AND md.manifest_sha256 = ?
		  AND md.dimension = ?
	`, identity.ModelID, identity.ModelRevision, identity.ManifestSHA256, identity.Dimension).Scan(&health.Indexed); err != nil {
		return IndexHealth{}, fmt.Errorf("count indexed memories: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM memories m
		LEFT JOIN memory_embeddings e ON e.rowid = m.id
		LEFT JOIN memory_embedding_metadata md ON md.memory_id = m.id
		WHERE e.rowid IS NULL OR md.memory_id IS NULL
	`).Scan(&health.MissingEmbeddings); err != nil {
		return IndexHealth{}, fmt.Errorf("count missing embeddings: %w", err)
	}
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM memories m
		JOIN memory_embedding_metadata md ON md.memory_id = m.id
		WHERE md.model_id != ?
		   OR md.model_revision != ?
		   OR md.manifest_sha256 != ?
		   OR md.dimension != ?
	`, identity.ModelID, identity.ModelRevision, identity.ManifestSHA256, identity.Dimension).Scan(&health.Stale); err != nil {
		return IndexHealth{}, fmt.Errorf("count stale embeddings: %w", err)
	}
	return health, nil
}

func upsertEmbedding(ctx context.Context, tx *sql.Tx, id int64, embedding []float32, identity EmbeddingIdentity) error {
	blob, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return fmt.Errorf("serialize embedding: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_embeddings WHERE rowid = ?`, id); err != nil {
		return fmt.Errorf("delete previous embedding: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO memory_embeddings(rowid, embedding) VALUES (?, ?)`, id, blob); err != nil {
		return fmt.Errorf("insert embedding: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO memory_embedding_metadata (
			memory_id, model_id, model_revision, manifest_sha256, dimension, indexed_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(memory_id) DO UPDATE SET
			model_id = excluded.model_id,
			model_revision = excluded.model_revision,
			manifest_sha256 = excluded.manifest_sha256,
			dimension = excluded.dimension,
			indexed_at = excluded.indexed_at
	`, id, identity.ModelID, identity.ModelRevision, identity.ManifestSHA256, identity.Dimension, time.Now().UTC().UnixMilli()); err != nil {
		return fmt.Errorf("record embedding metadata: %w", err)
	}
	return nil
}

func getMemoryTx(ctx context.Context, tx *sql.Tx, id int64) (domain.Memory, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, text, created_at, updated_at
		FROM memories
		WHERE id = ?
	`, id)

	var (
		memory                         domain.Memory
		createdUnixMillis, updatedUnix int64
	)

	err := row.Scan(&memory.ID, &memory.Text, &createdUnixMillis, &updatedUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Memory{}, ErrMemoryNotFound
	}
	if err != nil {
		return domain.Memory{}, fmt.Errorf("fetch memory %d: %w", id, err)
	}

	memory.CreatedAt = time.UnixMilli(createdUnixMillis).UTC()
	memory.UpdatedAt = time.UnixMilli(updatedUnix).UTC()
	return memory, nil
}

func scanMemory(rows *sql.Rows) (domain.Memory, error) {
	var (
		memory                         domain.Memory
		createdUnixMillis, updatedUnix int64
	)
	if err := rows.Scan(&memory.ID, &memory.Text, &createdUnixMillis, &updatedUnix); err != nil {
		return domain.Memory{}, fmt.Errorf("scan memory row: %w", err)
	}
	memory.CreatedAt = time.UnixMilli(createdUnixMillis).UTC()
	memory.UpdatedAt = time.UnixMilli(updatedUnix).UTC()
	return memory, nil
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

func buildFTSQuery(query string) string {
	tokens := tokenizeFTSQuery(query)
	if len(tokens) == 0 {
		return ""
	}
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

func fuzzyScore(query string, text string) int {
	needle := strings.ToLower(strings.TrimSpace(query))
	haystack := strings.ToLower(text)
	if needle == "" || haystack == "" {
		return -1
	}

	if idx := strings.Index(haystack, needle); idx >= 0 {
		return 100000 - len([]rune(haystack[:idx]))
	}

	needleRunes := compactSpaceRunes(needle)
	haystackRunes := compactSpaceRunes(haystack)
	if len(needleRunes) == 0 || len(haystackRunes) == 0 {
		return -1
	}

	pos := 0
	gaps := 0
	for _, want := range needleRunes {
		next := indexRune(haystackRunes[pos:], want)
		if next < 0 {
			return -1
		}
		gaps += next
		pos += next + 1
	}
	return 5000 - gaps
}

func compactSpaceRunes(value string) []rune {
	out := make([]rune, 0, len(value))
	for _, r := range value {
		if unicode.IsSpace(r) {
			continue
		}
		out = append(out, r)
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

func clampLimit(value int, defaultValue int, maxValue int) int {
	if value <= 0 {
		return defaultValue
	}
	return min(value, maxValue)
}
