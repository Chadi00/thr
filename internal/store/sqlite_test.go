package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenExistingReturnsDatabaseNotFound(t *testing.T) {
	_, err := OpenExisting(filepath.Join(t.TempDir(), "missing.db"))
	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Fatalf("expected ErrDatabaseNotFound, got %v", err)
	}
}

func TestOpenCreatesPrivateDatabaseFiles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "thr.db")
	db, err := Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	assertPathMode(t, dbPath, 0o600)
	if _, err := db.Exec(`INSERT INTO scoped_memories(text, scope_id, scope_assignment, created_at, updated_at, scope_updated_at) VALUES ('x', 'user', 'explicit', 1, 1, 1)`); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	assertPathMode(t, dbPath+"-wal", 0o600)
	assertPathMode(t, dbPath+"-shm", 0o600)
}

func TestOpenExistingMigratesWritableDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old.db")
	createLegacyDatabase(t, dbPath)

	migrated, err := OpenExisting(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatalf("open existing: %v", err)
	}
	defer migrated.Close()

	var exists int
	if err := migrated.QueryRow(`SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'scoped_memory_embedding_metadata'`).Scan(&exists); err != nil {
		t.Fatalf("expected metadata table after auto migration: %v", err)
	}
}

func TestOpenExistingReadOnlyPendingMigrationReturnsFriendlyError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "readonly-old.db")
	createLegacyDatabase(t, dbPath)
	chmodWithRestoreStore(t, dbPath, 0o400)
	chmodWithRestoreStore(t, dir, 0o500)

	_, err := OpenExisting(dbPath)
	if !errors.Is(err, ErrMigrationRequired) {
		t.Fatalf("expected migration required error, got %v", err)
	}
}

func TestSQLiteDSNEscapesPathSeparators(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "space ? # %.db")
	db, err := Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatalf("open db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected sqlite database at literal path: %v", err)
	}
}

func TestReadOnlyInspectionDoesNotCreateMissingSharedMemory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "thr.db")
	db, err := Open(dbPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath+"-wal", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectDatabase(dbPath); !errors.Is(err, ErrReadOnlyWALUnavailable) {
		t.Fatalf("expected conservative WAL error, got %v", err)
	}
	if _, err := os.Stat(dbPath + "-shm"); !os.IsNotExist(err) {
		t.Fatalf("inspection created shared memory: %v", err)
	}
}

func assertPathMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s: got %o want %o", path, got, want)
	}
}

func chmodWithRestoreStore(t *testing.T, path string, mode os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	original := info.Mode().Perm()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(path, original)
	})
}
