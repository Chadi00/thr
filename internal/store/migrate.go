package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"sort"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY NOT NULL,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
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

		contents, err := migrationFiles.ReadFile("migrations/" + name)
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
