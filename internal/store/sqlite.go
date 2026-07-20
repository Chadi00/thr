package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/Chadi00/thr/internal/privacy"
	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

var (
	ErrDatabaseNotFound       = errors.New("database not found")
	ErrReadOnlyWALUnavailable = errors.New("read-only WAL inspection unavailable")
)

func Open(path string) (*sql.DB, error) {
	info, err := InspectOperationalDatabase(path)
	if err != nil {
		return nil, err
	}
	switch info.Status {
	case DatabaseMissing:
		return CreateCurrentDatabase(path)
	case DatabaseMigrationRequired:
		if err := migrateAutomatically(path); err != nil {
			return nil, err
		}
	case DatabaseIncompatible:
		if _, err := os.Stat(path + ".create.lock"); err == nil {
			return CreateCurrentDatabase(path)
		}
		return nil, ErrDatabaseIncompatible
	}
	return openCurrentWritable(path)
}

func OpenExistingWritable(path string) (*sql.DB, error) {
	info, err := InspectOperationalDatabase(path)
	if err != nil {
		return nil, err
	}
	switch info.Status {
	case DatabaseMissing:
		return nil, ErrDatabaseNotFound
	case DatabaseMigrationRequired:
		if err := migrateAutomatically(path); err != nil {
			return nil, err
		}
	case DatabaseIncompatible:
		return nil, ErrDatabaseIncompatible
	}
	return openCurrentWritable(path)
}

func OpenExisting(path string) (*sql.DB, error) {
	info, err := InspectOperationalDatabase(path)
	if err != nil {
		return nil, err
	}
	switch info.Status {
	case DatabaseMissing:
		return nil, ErrDatabaseNotFound
	case DatabaseMigrationRequired:
		if err := migrateAutomatically(path); err != nil {
			return nil, err
		}
	case DatabaseIncompatible:
		return nil, ErrDatabaseIncompatible
	}

	db, err := openRaw(path, map[string]string{
		"mode":          "rw",
		"_foreign_keys": "on",
		"_busy_timeout": "5000",
		"_query_only":   "on",
	})
	if err == nil {
		return db, nil
	}
	db, err = openRaw(path, map[string]string{
		"mode":          "ro",
		"_foreign_keys": "on",
		"_query_only":   "on",
	})
	if err == nil {
		return db, nil
	}
	return openRaw(path, map[string]string{"mode": "ro", "immutable": "1", "_foreign_keys": "on", "_query_only": "on"})
}

func OpenCompatibleReadOnly(path string) (*sql.DB, error) {
	info, err := InspectDatabase(path)
	if err != nil {
		return nil, err
	}
	switch info.Status {
	case DatabaseMissing:
		return nil, ErrDatabaseNotFound
	case DatabaseMigrationRequired:
		return nil, ErrMigrationRequired
	case DatabaseIncompatible:
		return nil, ErrDatabaseIncompatible
	}
	return openReadOnly(path)
}

func openReadOnly(path string) (*sql.DB, error) {
	if _, err := os.Stat(path + "-wal"); err == nil {
		if _, err := os.Stat(path + "-shm"); errors.Is(err, os.ErrNotExist) {
			return nil, ErrReadOnlyWALUnavailable
		}
		db, err := openRaw(path, map[string]string{"mode": "ro", "_foreign_keys": "on", "_query_only": "on"})
		if err == nil {
			return db, nil
		}
	}
	return openRaw(path, map[string]string{"mode": "ro", "immutable": "1", "_foreign_keys": "on", "_query_only": "on"})
}

func migrateAutomatically(path string) error {
	if _, err := MigratePath(context.Background(), path); err != nil {
		return fmt.Errorf("%w: %v", ErrMigrationRequired, err)
	}
	return nil
}

func openCurrentWritable(path string) (*sql.DB, error) {
	db, err := openRaw(path, map[string]string{
		"mode":          "rw",
		"_foreign_keys": "on",
		"_busy_timeout": "5000",
		"_txlock":       "immediate",
	})
	if err != nil {
		return nil, err
	}
	if err := privacy.HardenSQLiteFiles(path); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func openRaw(path string, params map[string]string) (*sql.DB, error) {
	sqlite_vec.Auto()
	db, err := sql.Open("sqlite3", sqliteDSN(path, params))
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	return db, nil
}

func applyPragmas(db *sql.DB) error {
	for _, query := range []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA temp_store=MEMORY;",
	} {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("apply pragma %q: %w", query, err)
		}
	}
	return nil
}

func sqliteDSN(path string, params map[string]string) string {
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	return (&url.URL{Scheme: "file", Path: path, RawQuery: values.Encode()}).String()
}

func IsMigrationRequired(err error) bool { return errors.Is(err, ErrMigrationRequired) }
func IsDatabaseNotFound(err error) bool  { return errors.Is(err, ErrDatabaseNotFound) }
