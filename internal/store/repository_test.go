package store

import (
	"context"
	"fmt"
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

	identity := testEmbeddingIdentity()

	m1, err := repo.AddMemory(ctx, "the user likes sports cars", vectorA, identity)
	if err != nil {
		t.Fatalf("add m1: %v", err)
	}
	m2, err := repo.AddMemory(ctx, "the user prefers motorcycles", vectorB, identity)
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

	semanticHits, err := repo.SemanticSearch(ctx, vectorA, 1, identity)
	if err != nil {
		t.Fatalf("semantic search: %v", err)
	}
	if len(semanticHits) != 1 || semanticHits[0].Memory.ID != m1.ID {
		t.Fatalf("expected top semantic hit to be %d, got %+v", m1.ID, semanticHits)
	}

	updated, err := repo.EditMemory(ctx, m2.ID, "the user prefers classic sports cars", vectorA, identity)
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

	count, err := repo.CountMemories(ctx)
	if err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 memory after forget, got %d", count)
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
	memory, err := repo.AddMemory(ctx, "rust ownership tips", vectorOf(0.3), testEmbeddingIdentity())
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
	memory, err := repo.AddMemory(ctx, "literal 100%_match and more", vector, testEmbeddingIdentity())
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

func TestKeywordSearchTreatsInputAsPlainText(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "thr-fts-plain-text.db")
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
	if _, err := repo.AddMemory(ctx, `she said "quoted" syntax literally`, vectorOf(0.4), testEmbeddingIdentity()); err != nil {
		t.Fatalf("add memory: %v", err)
	}

	hits, err := repo.KeywordSearch(ctx, `"quoted" OR NEAR(*)`, 5)
	if err != nil {
		t.Fatalf("keyword search with literal-looking FTS syntax: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected one hit, got %+v", hits)
	}
}

func TestRecallSearchDoesNotHideFTSFailures(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "thr-fts-failure.db")
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
	if _, err := repo.AddMemory(ctx, "semantic recall needs a working fts index", vectorOf(0.5), testEmbeddingIdentity()); err != nil {
		t.Fatalf("add memory: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE memory_fts`); err != nil {
		t.Fatalf("drop memory_fts: %v", err)
	}

	if _, err := repo.RecallSearch(ctx, "semantic", 5, 50, 25); err == nil {
		t.Fatal("expected recall search to surface the FTS failure")
	}
}

func TestIndexHealthTracksStaleAndMissingEmbeddings(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "thr-index-health.db")
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
	active := testEmbeddingIdentity()
	stale := EmbeddingIdentity{
		ModelID:        "old-model",
		ModelRevision:  "old-revision",
		ManifestSHA256: "old-manifest",
		Dimension:      768,
	}
	if _, err := repo.AddMemory(ctx, "fresh", vectorOf(0.1), active); err != nil {
		t.Fatalf("add fresh memory: %v", err)
	}
	staleMemory, err := repo.AddMemory(ctx, "stale", vectorOf(0.2), stale)
	if err != nil {
		t.Fatalf("add stale memory: %v", err)
	}
	missing, err := repo.AddMemory(ctx, "missing", vectorOf(0.3), active)
	if err != nil {
		t.Fatalf("add missing memory: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM memory_embeddings WHERE rowid = ?`, missing.ID); err != nil {
		t.Fatalf("delete vector row: %v", err)
	}

	health, err := repo.IndexHealth(ctx, active)
	if err != nil {
		t.Fatalf("index health: %v", err)
	}
	if health.Memories != 3 || health.Indexed != 1 || health.Stale != 1 || health.MissingEmbeddings != 1 {
		t.Fatalf("unexpected health: %+v", health)
	}

	needsIndex, err := repo.ListMemoriesNeedingIndex(ctx, active)
	if err != nil {
		t.Fatalf("list memories needing index: %v", err)
	}
	got := map[int64]bool{}
	for _, memory := range needsIndex {
		got[memory.ID] = true
	}
	if !got[staleMemory.ID] || !got[missing.ID] || len(got) != 2 {
		t.Fatalf("unexpected memories needing index: %+v", needsIndex)
	}

	if err := repo.UpsertMemoryEmbedding(ctx, staleMemory.ID, vectorOf(0.4), active); err != nil {
		t.Fatalf("update stale embedding: %v", err)
	}
	if err := repo.UpsertMemoryEmbedding(ctx, missing.ID, vectorOf(0.5), active); err != nil {
		t.Fatalf("update missing embedding: %v", err)
	}
	health, err = repo.IndexHealth(ctx, active)
	if err != nil {
		t.Fatalf("index health after repair: %v", err)
	}
	if health.Indexed != 3 || health.Stale != 0 || health.MissingEmbeddings != 0 {
		t.Fatalf("unexpected repaired health: %+v", health)
	}
}

func vectorOf(value float32) []float32 {
	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = value
	}
	return vec
}

func testEmbeddingIdentity() EmbeddingIdentity {
	return EmbeddingIdentity{
		ModelID:        "test-model",
		ModelRevision:  "test-revision",
		ManifestSHA256: "test-manifest",
		Dimension:      768,
	}
}

func BenchmarkRecallSearch(b *testing.B) {
	ctx := context.Background()
	dbPath := filepath.Join(b.TempDir(), "thr-recall-bench.db")
	db, err := Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			b.Skip("sqlite build does not include fts5")
		}
		b.Fatalf("open db: %v", err)
	}
	defer db.Close()

	repo := NewRepository(db)
	identity := testEmbeddingIdentity()
	for i := 0; i < 2500; i++ {
		text := fmt.Sprintf("memory %04d about project notes, semantic search, privacy, and indexed recall", i)
		if _, err := repo.AddMemory(ctx, text, vectorOf(float32(i%10)/10), identity); err != nil {
			b.Fatalf("add memory: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := repo.RecallSearch(ctx, "privacy indexed recall", 25, DefaultRecentWindow, MaxRecallCandidates); err != nil {
			b.Fatalf("recall search: %v", err)
		}
	}
}
