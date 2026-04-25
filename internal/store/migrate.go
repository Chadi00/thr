package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var ErrMigrationRequired = errors.New("database needs an automatic update but is not writable")

func Migrate(db *sql.DB) error {
	return migrate(db, migrationFiles)
}

func CheckCompatible(db *sql.DB) error {
	pending, err := pendingMigrations(db, migrationFiles)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		return fmt.Errorf("%w", ErrMigrationRequired)
	}
	return nil
}

type migrationReader interface {
	fs.ReadDirFS
	fs.ReadFileFS
}

func migrate(db *sql.DB, files migrationReader) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY NOT NULL,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	entries, err := files.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var exists int
		err := db.QueryRow(`SELECT 1 FROM schema_migrations WHERE name = ?`, name).Scan(&exists)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check migration %s status: %w", name, err)
		}

		contents, err := files.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s transaction: %w", name, err)
		}

		if _, err := tx.Exec(string(contents)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("run migration %s: %w", name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(name, applied_at) VALUES (?, ?)`,
			name,
			time.Now().UTC().UnixMilli(),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s transaction: %w", name, err)
		}
	}

	return nil
}

func pendingMigrations(db *sql.DB, files migrationReader) ([]string, error) {
	names, err := migrationNames(files)
	if err != nil {
		return nil, err
	}

	if !schemaMigrationsExists(db) {
		return names, nil
	}

	applied := make(map[string]struct{}, len(names))
	rows, err := db.Query(`SELECT name FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan schema migration: %w", err)
		}
		applied[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema migrations: %w", err)
	}

	pending := make([]string, 0)
	for _, name := range names {
		if _, exists := applied[name]; !exists {
			pending = append(pending, name)
		}
	}
	return pending, nil
}

func schemaMigrationsExists(db *sql.DB) bool {
	var exists int
	err := db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&exists)
	return err == nil
}

func migrationNames(files migrationReader) ([]string, error) {
	entries, err := files.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}
