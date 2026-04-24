package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Chadi00/thr/internal/domain"
	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

var ErrMemoryNotFound = errors.New("memory not found")

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

func (r *Repository) AddMemory(ctx context.Context, text string, embedding []float32) (domain.Memory, error) {
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

	if err := upsertEmbedding(ctx, tx, memoryID, embedding); err != nil {
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
	if limit <= 0 {
		limit = 100
	}

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

func (r *Repository) EditMemory(ctx context.Context, id int64, text string, embedding []float32) (domain.Memory, error) {
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

	if err := upsertEmbedding(ctx, tx, id, embedding); err != nil {
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

func (r *Repository) ImportMemory(ctx context.Context, text string, createdAt time.Time, updatedAt time.Time, embedding []float32) (domain.Memory, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Memory{}, fmt.Errorf("begin import memory transaction: %w", err)
	}
	defer rollback(tx)

	res, err := tx.ExecContext(ctx, `
		INSERT INTO memories (text, created_at, updated_at)
		VALUES (?, ?, ?)
	`, text, createdAt.UnixMilli(), updatedAt.UnixMilli())
	if err != nil {
		return domain.Memory{}, fmt.Errorf("import memory row: %w", err)
	}

	memoryID, err := res.LastInsertId()
	if err != nil {
		return domain.Memory{}, fmt.Errorf("get imported memory id: %w", err)
	}

	if err := upsertEmbedding(ctx, tx, memoryID, embedding); err != nil {
		return domain.Memory{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Memory{}, fmt.Errorf("commit import memory transaction: %w", err)
	}

	return domain.Memory{
		ID:        memoryID,
		Text:      text,
		CreatedAt: createdAt.UTC(),
		UpdatedAt: updatedAt.UTC(),
	}, nil
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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit forget memory transaction: %w", err)
	}

	return nil
}

func (r *Repository) SemanticSearch(ctx context.Context, embedding []float32, limit int) ([]SemanticHit, error) {
	if limit <= 0 {
		limit = 3
	}
	query, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query embedding: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.text, m.created_at, m.updated_at, e.distance
		FROM memory_embeddings e
		JOIN memories m ON m.id = e.rowid
		WHERE e.embedding MATCH ? AND e.k = ?
		ORDER BY e.distance
	`, query, limit)
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
		hits = append(hits, SemanticHit{Memory: memory, Distance: distance})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate semantic hits: %w", err)
	}

	return hits, nil
}

func (r *Repository) KeywordSearch(ctx context.Context, query string, limit int) ([]domain.Memory, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT m.id, m.text, m.created_at, m.updated_at
		FROM memory_fts f
		JOIN memories m ON m.id = f.rowid
		WHERE memory_fts MATCH ?
		ORDER BY bm25(memory_fts)
		LIMIT ?
	`, query, limit)
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

func (r *Repository) SubstringSearch(ctx context.Context, query string, limit int) ([]domain.Memory, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, text, created_at, updated_at
		FROM memories
		WHERE text LIKE ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, "%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("substring search query: %w", err)
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
		return nil, fmt.Errorf("iterate substring results: %w", err)
	}
	return results, nil
}

func (r *Repository) loadRecentWindow(ctx context.Context, recentWindow int) ([]domain.Memory, error) {
	if recentWindow <= 0 {
		recentWindow = 2000
	}
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

func (r *Repository) SubstringSearchRecent(ctx context.Context, query string, recentWindow int, limit int) ([]domain.Memory, error) {
	if limit <= 0 {
		limit = 10
	}
	recent, err := r.loadRecentWindow(ctx, recentWindow)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	if needle == "" {
		return nil, nil
	}
	results := make([]domain.Memory, 0, min(limit, len(recent)))
	for _, m := range recent {
		if len(results) >= limit {
			break
		}
		if strings.Contains(strings.ToLower(m.Text), needle) {
			results = append(results, m)
		}
	}
	return results, nil
}

func (r *Repository) RecallSearch(ctx context.Context, query string, limit int, recentWindow int, candidateLimit int) ([]domain.Memory, error) {
	if limit <= 0 {
		limit = 10
	}
	if recentWindow <= 0 {
		recentWindow = 2000
	}
	if candidateLimit <= 0 {
		candidateLimit = max(limit*8, 64)
	}

	candidates := make([]domain.Memory, 0, candidateLimit)
	seen := make(map[int64]struct{}, candidateLimit)

	ftsQuery := buildFTSQuery(query)
	if ftsQuery != "" {
		ftsHits, err := r.KeywordSearch(ctx, ftsQuery, candidateLimit)
		if err == nil {
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
		}
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
		if hit.score < 0 {
			continue
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

func (r *Repository) Vacuum(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("vacuum database: %w", err)
	}
	return nil
}

func upsertEmbedding(ctx context.Context, tx *sql.Tx, id int64, embedding []float32) error {
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
	tokens := strings.Fields(strings.ToLower(query))
	if len(tokens) == 0 {
		return ""
	}
	clauses := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.Trim(token, "\"'`.,!?;:()[]{}")
		if token == "" {
			continue
		}
		clauses = append(clauses, fmt.Sprintf("(%s OR %s*)", token, token))
	}
	return strings.Join(clauses, " OR ")
}

func fuzzyScore(query string, text string) int {
	needle := strings.ToLower(strings.TrimSpace(query))
	haystack := strings.ToLower(text)
	if needle == "" || haystack == "" {
		return -1
	}

	if idx := strings.Index(haystack, needle); idx >= 0 {
		return 100000 - idx
	}

	needle = strings.ReplaceAll(needle, " ", "")
	haystackCompact := strings.ReplaceAll(haystack, " ", "")
	if needle == "" || haystackCompact == "" {
		return -1
	}

	pos := 0
	gaps := 0
	for i := 0; i < len(needle); i++ {
		next := strings.IndexByte(haystackCompact[pos:], needle[i])
		if next < 0 {
			return -1
		}
		gaps += next
		pos += next + 1
	}
	return 5000 - gaps
}
