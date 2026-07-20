package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Chadi00/thr/internal/repoctx"
	"github.com/Chadi00/thr/internal/store"
)

func TestVersionFlagMatchesVersionCommand(t *testing.T) {
	flagOutput := runRootCommand(t, "--version")
	commandOutput := runRootCommand(t, "version")
	if flagOutput != commandOutput {
		t.Fatalf("expected matching version output, got flag=%q command=%q", flagOutput, commandOutput)
	}
}

func TestCompletionCommandIsDisabled(t *testing.T) {
	helpOutput := runRootCommand(t, "--help")
	if strings.Contains(helpOutput, "completion") {
		t.Fatalf("expected help output to omit completion command, got %q", helpOutput)
	}

	err := executeRootCommand("completion")
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected completion command to be unavailable, got %v", err)
	}
}

func TestStatsJSONOnMissingDatabaseDoesNotCreateDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	output := runRootCommand(t, "--db", dbPath, "--json", "stats")

	var stats map[string]any
	if err := json.Unmarshal([]byte(output), &stats); err != nil {
		t.Fatalf("decode stats json: %v", err)
	}
	if got := stats["db_path"]; got != dbPath {
		t.Fatalf("expected db_path %q, got %#v", dbPath, got)
	}
	if got := stats["memories"]; got != float64(0) {
		t.Fatalf("expected 0 memories, got %#v", got)
	}
	for _, key := range []string{"model_id", "model_revision", "model_manifest_sha256", "model_verified", "indexed_memories", "stale_memories", "missing_embeddings"} {
		if _, ok := stats[key]; !ok {
			t.Fatalf("expected stats json to include %q: %#v", key, stats)
		}
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("expected stats to leave missing db absent, stat err=%v", err)
	}
}

func TestListOnMissingDatabaseDoesNotCreateDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	output := runRootCommand(t, "--db", dbPath, "list")
	if strings.TrimSpace(output) != "no memories stored" {
		t.Fatalf("unexpected list output: %q", output)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("expected list to leave missing db absent, stat err=%v", err)
	}
}

func TestListCountFlagsLimitNewestMemories(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "thr.db")
	db, err := store.Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatalf("open db: %v", err)
	}

	base := time.Now().UTC().UnixMilli()
	for i := 1; i <= 5; i++ {
		if _, err := db.Exec(
			`INSERT INTO scoped_memories (text, scope_id, scope_assignment, created_at, updated_at, scope_updated_at) VALUES (?, 'user', 'explicit', ?, ?, ?)`,
			fmt.Sprintf("memory %d", i),
			base+int64(i),
			base+int64(i),
			base+int64(i),
		); err != nil {
			_ = db.Close()
			t.Fatalf("insert memory %d: %v", i, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	tests := [][]string{
		{"list", "--last", "4"},
		{"list", "--limit", "4"},
		{"list", "-n", "4"},
	}
	for _, args := range tests {
		output := runRootCommand(t, append([]string{"--db", dbPath}, args...)...)
		for _, want := range []string{"memory 5", "memory 4", "memory 3", "memory 2"} {
			if !strings.Contains(output, want) {
				t.Fatalf("expected %v output to include %q, got %q", args, want, output)
			}
		}
		if strings.Contains(output, "memory 1") {
			t.Fatalf("expected %v to omit oldest memory, got %q", args, output)
		}
	}
}

func TestShowOnMissingDatabaseReturnsNotFound(t *testing.T) {
	root := NewRootCommand("dev", "commit", "date")
	stdout := new(bytes.Buffer)
	root.SetOut(stdout)
	root.SetErr(stdout)
	root.SetArgs([]string{"--db", filepath.Join(t.TempDir(), "missing.db"), "show", "1"})
	err := root.ExecuteContext(context.Background())
	if err == nil || err.Error() != "memory 1 not found" {
		t.Fatalf("expected memory not found, got %v", err)
	}
}

func TestInvalidAddInputDoesNotCreateDBOrModelCache(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	modelCache := filepath.Join(t.TempDir(), "models")
	t.Setenv("THR_MODEL_CACHE", modelCache)

	err := executeRootCommand("--db", dbPath, "add", "--max-bytes", "3", "abcd")
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected too large error, got %v", err)
	}
	assertPathAbsent(t, dbPath)
	assertPathAbsent(t, modelCache)
}

func TestInvalidEditIDDoesNotCreateDBOrModelCache(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	modelCache := filepath.Join(t.TempDir(), "models")
	t.Setenv("THR_MODEL_CACHE", modelCache)

	err := executeRootCommand("--db", dbPath, "edit", "nope", "replacement")
	if err == nil || !strings.Contains(err.Error(), "invalid id") {
		t.Fatalf("expected invalid id error, got %v", err)
	}
	assertPathAbsent(t, dbPath)
	assertPathAbsent(t, modelCache)
}

func TestInvalidForgetIDDoesNotCreateDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")

	err := executeRootCommand("--db", dbPath, "forget", "nope")
	if err == nil || !strings.Contains(err.Error(), "invalid id") {
		t.Fatalf("expected invalid id error, got %v", err)
	}
	assertPathAbsent(t, dbPath)
}

func TestIndexOnMissingDatabaseDoesNotCreateDBOrModelCache(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	modelCache := filepath.Join(t.TempDir(), "models")
	t.Setenv("THR_MODEL_CACHE", modelCache)

	output := runRootCommand(t, "--db", dbPath, "index")
	if strings.TrimSpace(output) != "no memories stored" {
		t.Fatalf("unexpected index output: %q", output)
	}
	assertPathAbsent(t, dbPath)
	assertPathAbsent(t, modelCache)
}

func TestAskOnMissingDatabaseDoesNotCreateDBOrModelCache(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	modelCache := filepath.Join(t.TempDir(), "models")
	t.Setenv("THR_MODEL_CACHE", modelCache)

	output := runRootCommand(t, "--db", dbPath, "ask", "what does the user prefer?")
	if strings.TrimSpace(output) != "no matching memories" {
		t.Fatalf("unexpected ask output: %q", output)
	}
	assertPathAbsent(t, dbPath)
	assertPathAbsent(t, modelCache)
}

func TestAskRejectsInvalidMaxDistanceBeforeRuntimeInit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	modelCache := filepath.Join(t.TempDir(), "models")
	t.Setenv("THR_MODEL_CACHE", modelCache)

	err := executeRootCommand("--db", dbPath, "ask", "--max-distance", "0", "anything")
	if err == nil || !strings.Contains(err.Error(), "--max-distance must be greater than 0 and at most 4") {
		t.Fatalf("expected max-distance validation error, got %v", err)
	}
	assertPathAbsent(t, dbPath)
	assertPathAbsent(t, modelCache)
}

func TestContextProspectiveRepositoryIsReadOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	if output, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	output := runRootCommand(t, "--db", dbPath, "--cwd", repo, "--format", "json-v2", "context")
	var envelope struct {
		Context struct {
			Database struct {
				Status string `json:"status"`
			} `json:"database"`
			Prospective *struct {
				ID *string `json:"id"`
			} `json:"prospective_scope"`
			Resolution struct {
				Status string `json:"status"`
			} `json:"resolution"`
		} `json:"context"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Context.Database.Status != "missing" || envelope.Context.Prospective == nil || envelope.Context.Prospective.ID != nil || envelope.Context.Resolution.Status != "prospective" {
		t.Fatalf("unexpected context: %s", output)
	}
	assertPathAbsent(t, dbPath)
}

func TestContextDoesNotChangeCurrentDatabaseFiles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "thr.db")
	db, err := store.Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	runRootCommand(t, "--db", dbPath, "--cwd", dir, "context")
	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) || !info.ModTime().Equal(afterInfo.ModTime()) || info.Mode() != afterInfo.Mode() {
		t.Fatalf("context changed database files: before=%v after=%v", before, after)
	}
}

func TestUnqualifiedWriteOutsideRepositoryHasNoSideEffects(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	modelCache := filepath.Join(t.TempDir(), "models")
	t.Setenv("THR_MODEL_CACHE", modelCache)
	err := executeRootCommand("--db", dbPath, "--cwd", t.TempDir(), "add", "repository fact")
	if err == nil || !strings.Contains(err.Error(), "--scope user") {
		t.Fatalf("expected unresolved write scope, got %v", err)
	}
	assertPathAbsent(t, dbPath)
	assertPathAbsent(t, modelCache)
}

func TestEmptyJSONV2RecallReportsSearchedScope(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	output := runRootCommand(t, "--db", dbPath, "--cwd", t.TempDir(), "--format", "json-v2", "list", "--scope", "user")
	var envelope struct {
		Context struct {
			Selection struct {
				Resolved []string `json:"resolved"`
			} `json:"scope_selection"`
		} `json:"context"`
		Result struct {
			Memories []any `json:"memories"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Context.Selection.Resolved) != 1 || envelope.Context.Selection.Resolved[0] != "user" || envelope.Result.Memories == nil || len(envelope.Result.Memories) != 0 {
		t.Fatalf("unexpected empty v2 response: %s", output)
	}
}

func TestJSONV2ErrorIdentifiesOperationAndSelection(t *testing.T) {
	root := NewRootCommand("v1", "commit", "date")
	root.SetArgs([]string{"--db", filepath.Join(t.TempDir(), "missing.db"), "--cwd", t.TempDir(), "--format", "json-v2", "add", "repo fact"})
	executed, err := root.ExecuteContextC(context.Background())
	if err == nil {
		t.Fatal("expected write scope error")
	}
	buf := new(bytes.Buffer)
	if !PrintError(executed, err, buf) {
		t.Fatal("expected JSON-v2 error output")
	}
	var envelope struct {
		Command string `json:"command"`
		Context struct {
			Selection struct {
				Mode     string   `json:"mode"`
				Resolved []string `json:"resolved"`
			} `json:"scope_selection"`
		} `json:"context"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Command != "memory.add" || envelope.Error.Code != "write_scope_unresolved" || envelope.Context.Selection.Mode != "automatic_write" || envelope.Context.Selection.Resolved == nil {
		t.Fatalf("unexpected error envelope: %s", buf.String())
	}
}

func TestJSONV2ErrorKeepsResolvedScopes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "thr.db")
	db, err := store.Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO scoped_memories(text, scope_id, scope_assignment, created_at, updated_at, scope_updated_at) VALUES ('missing index', 'user', 'explicit', 1, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	root := NewRootCommand("v1", "commit", "date")
	root.SetArgs([]string{"--db", dbPath, "--cwd", t.TempDir(), "--format", "json-v2", "ask", "missing"})
	executed, err := root.ExecuteContextC(context.Background())
	if err == nil {
		t.Fatal("expected stale index error")
	}
	buf := new(bytes.Buffer)
	PrintError(executed, err, buf)
	var envelope struct {
		Context struct {
			Selection struct {
				Resolved []string `json:"resolved"`
			} `json:"scope_selection"`
		} `json:"context"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "index_stale" || len(envelope.Context.Selection.Resolved) != 1 || envelope.Context.Selection.Resolved[0] != "user" {
		t.Fatalf("resolved scopes lost from error: %s", buf.String())
	}
}

func TestExactShowBypassesRepositoryResolution(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "thr.db")
	db, err := store.Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO scoped_memories(id, text, scope_id, scope_assignment, created_at, updated_at, scope_updated_at) VALUES (42, 'exact memory', 'user', 'explicit', 1, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	output := runRootCommand(t, "--db", dbPath, "--cwd", filepath.Join(t.TempDir(), "missing"), "show", "42")
	if !strings.Contains(output, "exact memory") || !strings.Contains(output, "[user]") {
		t.Fatalf("unexpected exact show output: %q", output)
	}
}

func TestScopeFlagMatrixRejectsInvalidCombinations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"add repeated scope", []string{"--db", dbPath, "add", "--scope", "user", "--scope", "repo", "x"}, "exactly one --scope"},
		{"read scope and all", []string{"--db", dbPath, "list", "--scope", "user", "--all-scopes"}, "mutually exclusive"},
		{"show retrieval scope", []string{"--db", dbPath, "show", "--scope", "user", "1"}, "unknown flag"},
		{"scope independent", []string{"--db", dbPath, "prefetch", "--scope", "user"}, "unknown flag"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := executeRootCommand(test.args...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
	assertPathAbsent(t, dbPath)
}

func TestDefaultRepositoryListIncludesRepoAndUserOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := filepath.Join(t.TempDir(), "repo")
	if output, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	observation, err := repoctx.Observe(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "thr.db")
	db, err := store.Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatal(err)
	}
	repository := store.NewRepository(db)
	repoScope, err := repository.EnsureRepositoryScope(context.Background(), observation)
	if err != nil {
		t.Fatal(err)
	}
	otherScope, err := repository.EnsureRepositoryScope(context.Background(), repoctx.Observation{CommonDir: "/tmp/unrelated/.git", Label: "unrelated"})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ text, scope string }{{"repo memory", repoScope.ID}, {"user memory", "user"}, {"other memory", otherScope.ID}} {
		if _, err := db.Exec(`INSERT INTO scoped_memories(text, scope_id, scope_assignment, created_at, updated_at, scope_updated_at) VALUES (?, ?, 'explicit', 1, 1, 1)`, row.text, row.scope); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	output := runRootCommand(t, "--db", dbPath, "--cwd", root, "list")
	if !strings.Contains(output, "repo memory") || !strings.Contains(output, "user memory") || strings.Contains(output, "other memory") {
		t.Fatalf("default visibility was not repo + user: %q", output)
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, stat err=%v", path, err)
	}
}

func TestListOnReadOnlyDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "readonly.db")
	db, err := store.Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatalf("open db: %v", err)
	}

	now := time.Now().UTC().UnixMilli()
	if _, err := db.Exec(
		`INSERT INTO scoped_memories (text, scope_id, scope_assignment, created_at, updated_at, scope_updated_at) VALUES (?, 'user', 'explicit', ?, ?, ?)`,
		"read-only memory",
		now,
		now,
		now,
	); err != nil {
		_ = db.Close()
		t.Fatalf("insert memory: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	chmodWithRestore(t, filepath.Dir(dbPath), 0o555)
	chmodWithRestore(t, dbPath, 0o444)
	chmodWithRestore(t, dbPath+"-wal", 0o444)
	chmodWithRestore(t, dbPath+"-shm", 0o444)

	output := runRootCommand(t, "--db", dbPath, "list")
	if !strings.Contains(output, "read-only memory") {
		t.Fatalf("expected list output to include stored memory, got %q", output)
	}
}

func chmodWithRestore(t *testing.T, path string, mode os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("stat %s: %v", path, err)
	}
	originalMode := info.Mode().Perm()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(path, originalMode)
	})
}

func runRootCommand(t *testing.T, args ...string) string {
	t.Helper()
	root := NewRootCommand("v1.2.3", "abc123", "2026-04-24T00:00:00Z")
	stdout := new(bytes.Buffer)
	root.SetOut(stdout)
	root.SetErr(stdout)
	root.SetArgs(args)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return stdout.String()
}

func executeRootCommand(args ...string) error {
	root := NewRootCommand("v1.2.3", "abc123", "2026-04-24T00:00:00Z")
	stdout := new(bytes.Buffer)
	root.SetOut(stdout)
	root.SetErr(stdout)
	root.SetArgs(args)
	return root.ExecuteContext(context.Background())
}
