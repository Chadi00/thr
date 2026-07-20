package store

import (
	"context"
	"database/sql"
	"embed"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var legacyMigrationFiles embed.FS

func TestCurrentDatabaseIsScopedAndOldSQLFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thr.db")
	db, err := Open(path)
	if err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatal(err)
	}
	defer db.Close()

	var scope string
	if err := db.QueryRow(`SELECT id FROM scopes WHERE kind = 'user'`).Scan(&scope); err != nil || scope != "user" {
		t.Fatalf("user scope: %q, %v", scope, err)
	}
	if _, err := db.Exec(`INSERT INTO scoped_memories(text, scope_id, scope_assignment, created_at, updated_at, scope_updated_at) VALUES ('x', NULL, 'explicit', 1, 1, 1)`); err == nil {
		t.Fatal("expected null scope to fail")
	}
	if _, err := db.Exec(`DELETE FROM scopes WHERE id = 'user'`); err == nil {
		t.Fatal("expected scope deletion to fail")
	}
	if _, err := db.Exec(`SELECT * FROM memories`); err == nil {
		t.Fatal("legacy table remained readable")
	}
	var migrations int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&migrations); err != nil || migrations != 4 {
		t.Fatalf("legacy migration boundary: %d, %v", migrations, err)
	}
}

func TestMigratePathPreservesLegacyRowsAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thr.db")
	createLegacyDatabase(t, path)
	before, err := InspectDatabase(path)
	if err != nil || before.Status != DatabaseMigrationRequired {
		t.Fatalf("legacy inspection: %+v, %v", before, err)
	}

	result, err := MigratePath(context.Background(), path)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if result.Memories != 1 || result.Embeddings != 1 || result.EmbeddingModels != 1 {
		t.Fatalf("unexpected migration result: %+v", result)
	}
	if info, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("backup: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode: %v", info.Mode().Perm())
	}

	db, err := OpenExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id, created, updated int64
	var text, scope, assignment string
	if err := db.QueryRow(`SELECT id, text, scope_id, scope_assignment, created_at, updated_at FROM scoped_memories`).Scan(&id, &text, &scope, &assignment, &created, &updated); err != nil {
		t.Fatal(err)
	}
	if id != 7 || text != "legacy memory" || scope != "user" || assignment != "legacy_default" || created != 100 || updated != 200 {
		t.Fatalf("legacy memory changed: id=%d text=%q scope=%q assignment=%q created=%d updated=%d", id, text, scope, assignment, created, updated)
	}
	if _, err := db.Exec(`SELECT * FROM memories`); err == nil {
		t.Fatal("legacy table remained after migration")
	}
	var vectorRowID int64
	if err := db.QueryRow(`SELECT rowid FROM scoped_memory_embeddings`).Scan(&vectorRowID); err != nil || vectorRowID != 7 {
		t.Fatalf("vector row id changed: %d, %v", vectorRowID, err)
	}
	var ftsRowID int64
	if err := db.QueryRow(`SELECT rowid FROM scoped_memory_fts WHERE scoped_memory_fts MATCH 'legacy'`).Scan(&ftsRowID); err != nil || ftsRowID != 7 {
		t.Fatalf("migrated FTS row unavailable: %d, %v", ftsRowID, err)
	}

	backup, err := sql.Open("sqlite3", "file:"+result.BackupPath+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	if err := backup.QueryRow(`SELECT id FROM memories`).Scan(&id); err != nil || id != 7 {
		t.Fatalf("backup legacy row: %d, %v", id, err)
	}
}

func TestInspectMissingDatabaseHasNoSideEffects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "thr.db")
	info, err := InspectDatabase(path)
	if err != nil || info.Status != DatabaseMissing {
		t.Fatalf("inspect missing: %+v, %v", info, err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("inspection created parent directory: %v", err)
	}
}

func TestCurrentDatabaseCreationRollsBackPartialSchema(t *testing.T) {
	original := currentSchema
	currentSchema = `CREATE TABLE partial(id INTEGER); INVALID SQL;`
	t.Cleanup(func() { currentSchema = original })
	path := filepath.Join(t.TempDir(), "thr.db")
	if db, err := CreateCurrentDatabase(path); err == nil {
		db.Close()
		t.Fatal("expected schema creation failure")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("partial database survived failed creation: %v", err)
	}
}

func TestConcurrentMissingDatabaseCreationConverges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thr.db")
	errs := make([]error, 8)
	var wg sync.WaitGroup
	wg.Add(len(errs))
	for i := range errs {
		go func(i int) {
			defer wg.Done()
			db, err := Open(path)
			errs[i] = err
			if db != nil {
				_ = db.Close()
			}
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			if strings.Contains(err.Error(), "no such module: fts5") {
				t.Skip("sqlite build does not include fts5")
			}
			t.Fatalf("concurrent create: %v", err)
		}
	}
	info, err := InspectDatabase(path)
	if err != nil || info.Status != DatabaseCompatible {
		t.Fatalf("database did not converge: %+v, %v", info, err)
	}
}

func TestConcurrentAutomaticMigrationConverges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thr.db")
	createLegacyDatabase(t, path)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	for i := range errs {
		go func(i int) {
			defer wg.Done()
			_, errs[i] = MigratePath(context.Background(), path)
		}(i)
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("concurrent migrations: %v, %v", errs[0], errs[1])
	}
	info, err := InspectDatabase(path)
	if err != nil || info.Status != DatabaseCompatible {
		t.Fatalf("database did not converge: %+v, %v", info, err)
	}
}

func createLegacyDatabase(t *testing.T, path string) {
	t.Helper()
	sqlite_vec.Auto()
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations(name TEXT PRIMARY KEY NOT NULL, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_init.sql", "002_indexes.sql", "003_drop_schema_version.sql", "004_embedding_metadata.sql"} {
		contents, err := legacyMigrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(contents)); err != nil {
			if strings.Contains(err.Error(), "no such module: fts5") {
				t.Skip("sqlite build does not include fts5")
			}
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations(name, applied_at) VALUES (?, ?)`, name, time.Now().UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO memories(id, text, created_at, updated_at) VALUES (7, 'legacy memory', 100, 200)`); err != nil {
		t.Fatal(err)
	}
	vector, err := sqlite_vec.SerializeFloat32(make([]float32, 768))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO memory_embeddings(rowid, embedding) VALUES (7, ?)`, vector); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO memory_embedding_metadata(memory_id, model_id, model_revision, manifest_sha256, dimension, indexed_at) VALUES (7, 'm', 'r', 'd', 768, 300)`); err != nil {
		t.Fatal(err)
	}
}
