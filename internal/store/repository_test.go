package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryCRUDAndSearch(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "thr-test.db")
	db, err := Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
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
	got, err := repo.GetMemory(ctx, m1.ID)
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got.Text != m1.Text {
		t.Fatalf("unexpected get memory text: %q", got.Text)
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

	substringHits, err := repo.SubstringSearch(ctx, "classic sports", 5)
	if err != nil {
		t.Fatalf("substring search: %v", err)
	}
	if len(substringHits) != 1 || substringHits[0].ID != m2.ID {
		t.Fatalf("expected substring hit for memory %d, got %+v", m2.ID, substringHits)
	}

	count, err := repo.CountMemories(ctx)
	if err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 memory after forget, got %d", count)
	}

	imported, err := repo.ImportMemory(ctx, "imported memory", m1.CreatedAt, m1.UpdatedAt, vectorA)
	if err != nil {
		t.Fatalf("import memory: %v", err)
	}
	if imported.ID == 0 {
		t.Fatal("expected imported memory id to be set")
	}

	recallHits, err := repo.RecallSearch(ctx, "clasik sport", 5, 100, 100)
	if err != nil {
		t.Fatalf("recall search: %v", err)
	}
	if len(recallHits) == 0 || recallHits[0].ID != m2.ID {
		t.Fatalf("expected fuzzy recall hit for memory %d, got %+v", m2.ID, recallHits)
	}
}

func TestRecallSearchFuzzySubsequence(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "thr-fuzzy-subseq.db")
	db, err := Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewRepository(db)
	memory, err := repo.AddMemory(ctx, "rust ownership tips", vectorOf(0.3))
	if err != nil {
		t.Fatalf("add memory: %v", err)
	}
	// "rst" is not a contiguous substring of "rust" but matches as a subsequence.
	hits, err := repo.RecallSearch(ctx, "rst", 5, 200, 50)
	if err != nil {
		t.Fatalf("recall search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != memory.ID {
		t.Fatalf("expected subsequence match for 'rst' -> 'rust', got %+v", hits)
	}
}

func TestRecallSearchEscapesLikePattern(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "thr-like-test.db")
	db, err := Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewRepository(db)

	vector := vectorOf(0.2)
	memory, err := repo.AddMemory(ctx, "literal 100%_match and more", vector)
	if err != nil {
		t.Fatalf("add memory: %v", err)
	}

	hits, err := repo.RecallSearch(ctx, "100%_match", 5, 200, 50)
	if err != nil {
		t.Fatalf("recall search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != memory.ID {
		t.Fatalf("expected escaped LIKE hit for memory %d, got %+v", memory.ID, hits)
	}
}

func vectorOf(value float32) []float32 {
	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = value
	}
	return vec
}
