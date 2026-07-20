package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/repoctx"
)

func TestRepositoryScopesConvergeAndIsolateRecall(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "thr.db"))
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	ctx := context.Background()
	first := repoctx.Observation{CommonDir: "/tmp/clone-a/.git", WorktreeRoot: "/tmp/clone-a", CanonicalRemote: "github.com/acme/app", CanonicalizationVersion: 1, Label: "github.com/acme/app"}
	second := first
	second.CommonDir, second.WorktreeRoot = "/tmp/clone-b/.git", "/tmp/clone-b"

	var scopes [2]domain.Scope
	var errs [2]error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); scopes[0], errs[0] = repo.EnsureRepositoryScope(ctx, first) }()
	go func() { defer wg.Done(); scopes[1], errs[1] = repo.EnsureRepositoryScope(ctx, second) }()
	wg.Wait()
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("concurrent registration: %v, %v", errs[0], errs[1])
	}
	if scopes[0].ID != scopes[1].ID {
		t.Fatalf("matching clones split into %s and %s", scopes[0].ID, scopes[1].ID)
	}
	ambiguousBound := second
	ambiguousBound.CanonicalRemote = ""
	ambiguousBound.IdentityAmbiguous = true
	if resolution, err := repo.ResolveRepository(ctx, ambiguousBound); err != nil || resolution.Scope == nil || resolution.Scope.ID != scopes[0].ID || !resolution.Drift {
		t.Fatalf("bound checkout did not survive remote ambiguity: %+v, %v", resolution, err)
	}
	if _, _, err := repo.SplitRepository(ctx, first); err != nil {
		t.Fatalf("split repository: %v", err)
	}
	fresh := first
	fresh.CommonDir, fresh.WorktreeRoot = "/tmp/clone-c/.git", "/tmp/clone-c"
	if _, err := repo.ResolveRepository(ctx, fresh); !errors.Is(err, ErrRepositoryAmbiguous) {
		t.Fatalf("future clone after split should be ambiguous, got %v", err)
	}

	identity := testEmbeddingIdentity()
	repoMemory, err := repo.AddMemory(ctx, "repository-only needle", vectorOf(0.2), identity, scopes[0].ID, domain.ScopeAssignmentAutomatic)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddMemory(ctx, "user-wide needle", vectorOf(0.3), identity, "user", domain.ScopeAssignmentExplicit); err != nil {
		t.Fatal(err)
	}
	other := repoctx.Observation{CommonDir: "/tmp/other/.git", CanonicalRemote: "github.com/acme/other", CanonicalizationVersion: 1, Label: "github.com/acme/other"}
	otherScope, err := repo.EnsureRepositoryScope(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < DefaultRecallCandidateMin+10; i++ {
		if _, err := repo.AddMemory(ctx, "needle unrelated", vectorOf(0.1), identity, otherScope.ID, domain.ScopeAssignmentAutomatic); err != nil {
			t.Fatal(err)
		}
	}
	hits, err := repo.RecallSearch(ctx, "needle", []string{scopes[0].ID, "user"}, 10, 20, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Scope.ID == otherScope.ID || hits[1].Scope.ID == otherScope.ID {
		t.Fatalf("scope-isolated recall returned %+v", hits)
	}
	if hits[0].ID != repoMemory.ID && hits[1].ID != repoMemory.ID {
		t.Fatalf("repository memory missing from recall: %+v", hits)
	}
	semantic, err := repo.SemanticSearch(ctx, vectorOf(0.1), []string{scopes[0].ID, "user"}, 2, identity, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range semantic {
		if hit.Memory.Scope.ID == otherScope.ID {
			t.Fatalf("excluded scope entered semantic candidates: %+v", semantic)
		}
	}
	staleIdentity := identity
	staleIdentity.ModelRevision = "old"
	if _, err := repo.AddMemory(ctx, "stale unrelated", vectorOf(0.1), staleIdentity, otherScope.ID, domain.ScopeAssignmentAutomatic); err != nil {
		t.Fatal(err)
	}
	health, err := repo.IndexHealth(ctx, []string{scopes[0].ID, "user"}, identity)
	if err != nil || health.Stale != 0 {
		t.Fatalf("unrelated stale index contaminated selected health: %+v, %v", health, err)
	}
}

func TestMovePreservesContentTimestampAndEmbedding(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "thr.db"))
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	ctx := context.Background()
	observation := repoctx.Observation{CommonDir: "/tmp/repo/.git", Label: "repo"}
	scope, err := repo.EnsureRepositoryScope(ctx, observation)
	if err != nil {
		t.Fatal(err)
	}
	memory, err := repo.AddMemory(ctx, "move me", vectorOf(0.25), testEmbeddingIdentity(), scope.ID, domain.ScopeAssignmentAutomatic)
	if err != nil {
		t.Fatal(err)
	}
	var before []byte
	if err := db.QueryRow(`SELECT embedding FROM scoped_memory_embeddings WHERE rowid = ?`, memory.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	result, err := repo.MoveMemory(ctx, memory.ID, "user", Preconditions{ScopeID: scope.ID, Revision: memory.Revision})
	if err != nil {
		t.Fatal(err)
	}
	var after []byte
	if err := db.QueryRow(`SELECT embedding FROM scoped_memory_embeddings WHERE rowid = ? AND scope_id = 'user'`, memory.ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || !result.Memory.UpdatedAt.Equal(memory.UpdatedAt) || result.Memory.ID != memory.ID {
		t.Fatalf("move changed content identity: before=%+v after=%+v", memory, result.Memory)
	}
	if result.Memory.Scope.ID != "user" || result.Memory.ScopeAssignment != domain.ScopeAssignmentExplicit || result.Memory.Revision != memory.Revision+1 {
		t.Fatalf("move did not update scope metadata: %+v", result.Memory)
	}
	if result.FromAssignment != domain.ScopeAssignmentAutomatic {
		t.Fatalf("move lost source assignment: %q", result.FromAssignment)
	}
}

func TestMoveReportsLegacySourceAssignment(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "thr.db"))
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatal(err)
	}
	defer db.Close()
	result, err := db.Exec(`INSERT INTO scoped_memories(text, scope_id, scope_assignment, created_at, updated_at, scope_updated_at) VALUES ('legacy', 'user', 'legacy_default', 1, 1, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	moved, err := NewRepository(db).MoveMemory(context.Background(), id, "user", Preconditions{})
	if err != nil {
		t.Fatal(err)
	}
	if moved.FromAssignment != domain.ScopeAssignmentLegacy || moved.Memory.ScopeAssignment != domain.ScopeAssignmentExplicit {
		t.Fatalf("legacy move assignments: from=%q to=%q", moved.FromAssignment, moved.Memory.ScopeAssignment)
	}
}

func TestConcurrentFirstRepositoryWritesConverge(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "thr.db"))
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	observations := []repoctx.Observation{
		{CommonDir: "/tmp/write-a/.git", CanonicalRemote: "github.com/acme/write", CanonicalizationVersion: 1, Label: "github.com/acme/write"},
		{CommonDir: "/tmp/write-b/.git", CanonicalRemote: "github.com/acme/write", CanonicalizationVersion: 1, Label: "github.com/acme/write"},
	}
	memories := make([]domain.Memory, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := range observations {
		go func(i int) {
			defer wg.Done()
			memories[i], errs[i] = repo.AddRepositoryMemory(context.Background(), "first write", vectorOf(float32(i)), testEmbeddingIdentity(), observations[i], domain.ScopeAssignmentAutomatic)
		}(i)
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("concurrent first writes: %v, %v", errs[0], errs[1])
	}
	if memories[0].Scope.ID != memories[1].Scope.ID {
		t.Fatalf("first writes split scopes: %s, %s", memories[0].Scope.ID, memories[1].Scope.ID)
	}
}

func TestSemanticTiePrefersRepositoryScope(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "thr.db"))
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	ctx := context.Background()
	scope, err := repo.EnsureRepositoryScope(ctx, repoctx.Observation{CommonDir: "/tmp/tie/.git", Label: "tie"})
	if err != nil {
		t.Fatal(err)
	}
	identity := testEmbeddingIdentity()
	if _, err := repo.AddMemory(ctx, "same user", vectorOf(0.5), identity, "user", domain.ScopeAssignmentExplicit); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddMemory(ctx, "same repo", vectorOf(0.5), identity, scope.ID, domain.ScopeAssignmentExplicit); err != nil {
		t.Fatal(err)
	}
	hits, err := repo.SemanticSearch(ctx, vectorOf(0.5), []string{scope.ID, "user"}, 2, identity, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Memory.Scope.Kind != domain.ScopeKindRepo {
		t.Fatalf("semantic scope tie order: %+v", hits)
	}
}
