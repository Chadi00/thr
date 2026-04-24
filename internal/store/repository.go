package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/Chadi00/thr/internal/domain"
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
