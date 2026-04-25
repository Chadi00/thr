package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Chadi00/thr/internal/store"
)

func TestVersionFlagMatchesVersionCommand(t *testing.T) {
	flagOutput := runRootCommand(t, "--version")
	commandOutput := runRootCommand(t, "version")
	if flagOutput != commandOutput {
		t.Fatalf("expected matching version output, got flag=%q command=%q", flagOutput, commandOutput)
	}
}

func TestHelpDoesNotExpandCommandName(t *testing.T) {
	output := runRootCommand(t, "--help")
	forbidden := strings.Join([]string{"tiny", "history", "recall"}, " ")
	if strings.Contains(strings.ToLower(output), forbidden) {
		t.Fatalf("help output should not expand the command name: %s", output)
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
		`INSERT INTO memories (text, created_at, updated_at) VALUES (?, ?, ?)`,
		"read-only memory",
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
