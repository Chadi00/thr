package store

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func TestMigrateIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "thr-migrate.db")
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

	if err := Migrate(db); err != nil {
		t.Fatalf("rerun migrations: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 applied migrations, got %d", count)
	}
}

func TestMigrateDropsLegacySchemaVersionTable(t *testing.T) {
	sqlite_vec.Auto()
	dbPath := filepath.Join(t.TempDir(), "thr-legacy.db")
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if _, err := db.Exec(`CREATE TABLE schema_version(version INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create schema_version: %v", err)
	}

	if err := Migrate(db); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skip("sqlite build does not include fts5")
		}
		t.Fatalf("migrate db: %v", err)
	}

	var exists int
	err = db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'schema_version'`).Scan(&exists)
	if err != sql.ErrNoRows {
		t.Fatalf("expected schema_version to be removed, got exists=%d err=%v", exists, err)
	}
}

func TestMigrateRollsBackFailedMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "thr-migrate-failure.db")
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	files := fstest.MapFS{
		"migrations/001_good.sql": &fstest.MapFile{Data: []byte(`CREATE TABLE notes (id INTEGER PRIMARY KEY);`)},
		"migrations/002_bad.sql":  &fstest.MapFile{Data: []byte(`INSERT INTO notes(id) VALUES (1); INVALID SQL;`)},
	}

	if err := migrate(db, files); err == nil {
		t.Fatal("expected migration failure")
	}

	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}
	if applied != 1 {
		t.Fatalf("expected only the first migration to be recorded, got %d", applied)
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&rows); err != nil {
		t.Fatalf("count notes rows: %v", err)
	}
	if rows != 0 {
		t.Fatalf("expected failed migration insert to roll back, got %d rows", rows)
	}
}
