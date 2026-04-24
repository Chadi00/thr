package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

var ErrDatabaseNotFound = errors.New("database not found")

type openOptions struct {
	dsn        string
	initialize bool
}

func Open(path string) (*sql.DB, error) {
	return open(openOptions{
		dsn:        fmt.Sprintf("file:%s?_foreign_keys=on", path),
		initialize: true,
	})
}

func OpenExisting(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrDatabaseNotFound
		}
		return nil, fmt.Errorf("stat sqlite database: %w", err)
	}
	readOnly, err := shouldOpenImmutable(path)
	if err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_foreign_keys=on", path)
	if readOnly {
		dsn = fmt.Sprintf("file:%s?mode=ro&immutable=1&_foreign_keys=on", path)
	}
	return open(openOptions{
		dsn:        dsn,
		initialize: false,
	})
}

func open(options openOptions) (*sql.DB, error) {
	sqlite_vec.Auto()

	db, err := sql.Open("sqlite3", options.dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if !options.initialize {
		return db, nil
	}

	if err := applyPragmas(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := Migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA temp_store=MEMORY;",
	}

	for _, query := range pragmas {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("apply pragma %q: %w", query, err)
		}
	}
	return nil
}

func shouldOpenImmutable(path string) (bool, error) {
	fileReadOnly, err := lacksWritePermission(path)
	if err != nil {
		return false, fmt.Errorf("stat sqlite database: %w", err)
	}
	dirReadOnly, err := lacksWritePermission(filepath.Dir(path))
	if err != nil {
		return false, fmt.Errorf("stat sqlite database directory: %w", err)
	}
	return fileReadOnly || dirReadOnly, nil
}

func lacksWritePermission(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode().Perm()&0o222 == 0, nil
}
