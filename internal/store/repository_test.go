package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRepositoryCRUDAndSearch(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "thr-test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewRepository(db)

	vectorA := vectorOf(0.1)
	vectorB := vectorOf(0.9)

	m1, err := repo.AddMemory(ctx, "the user likes sports cars", vectorA)
	if err != nil {
		t.Fatalf("add m1: %v", err)
	}
	m2, err := repo.AddMemory(ctx, "the user prefers motorcycles", vectorB)
	if err != nil {
		t.Fatalf("add m2: %v", err)
	}

	memories, err := repo.ListMemories(ctx, 10)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(memories))
	}

	keywordHits, err := repo.KeywordSearch(ctx, "sports", 5)
	if err != nil {
		t.Fatalf("keyword search: %v", err)
	}
	if len(keywordHits) != 1 || keywordHits[0].ID != m1.ID {
		t.Fatalf("unexpected keyword hits: %+v", keywordHits)
	}

	semanticHits, err := repo.SemanticSearch(ctx, vectorA, 1)
	if err != nil {
		t.Fatalf("semantic search: %v", err)
	}
	if len(semanticHits) != 1 || semanticHits[0].Memory.ID != m1.ID {
		t.Fatalf("expected top semantic hit to be %d, got %+v", m1.ID, semanticHits)
	}

	updated, err := repo.EditMemory(ctx, m2.ID, "the user prefers classic sports cars", vectorA)
	if err != nil {
		t.Fatalf("edit memory: %v", err)
	}
	if updated.Text != "the user prefers classic sports cars" {
		t.Fatalf("unexpected updated text: %q", updated.Text)
	}

	keywordHits, err = repo.KeywordSearch(ctx, "classic", 5)
	if err != nil {
		t.Fatalf("keyword search after edit: %v", err)
	}
	if len(keywordHits) != 1 || keywordHits[0].ID != m2.ID {
		t.Fatalf("expected updated memory in keyword search, got %+v", keywordHits)
	}

	if err := repo.ForgetMemory(ctx, m1.ID); err != nil {
		t.Fatalf("forget memory: %v", err)
	}

	keywordHits, err = repo.KeywordSearch(ctx, "sports", 10)
	if err != nil {
		t.Fatalf("keyword search after forget: %v", err)
	}
	if len(keywordHits) != 1 || keywordHits[0].ID != m2.ID {
		t.Fatalf("expected only memory %d after forget, got %+v", m2.ID, keywordHits)
	}
}

func vectorOf(value float32) []float32 {
	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = value
	}
	return vec
}
