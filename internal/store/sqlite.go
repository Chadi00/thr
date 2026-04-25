package store

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/Chadi00/thr/internal/privacy"
	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

var ErrDatabaseNotFound = errors.New("database not found")

type openOptions struct {
	path       string
	dsn        string
	initialize bool
	readOnly   bool
}

func Open(path string) (*sql.DB, error) {
	if err := privacy.EnsurePrivateFile(path); err != nil {
		return nil, err
	}
	return open(openOptions{
		path:       path,
		dsn:        sqliteDSN(path, map[string]string{"_foreign_keys": "on"}),
		initialize: true,
	})
}

func OpenExistingWritable(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrDatabaseNotFound
		}
		return nil, fmt.Errorf("stat sqlite database: %w", err)
	}
	return Open(path)
}

func OpenExisting(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrDatabaseNotFound
		}
		return nil, fmt.Errorf("stat sqlite database: %w", err)
	}

	if canWriteDatabase(path) {
		db, err := open(openOptions{
			path:       path,
			dsn:        sqliteDSN(path, map[string]string{"mode": "rw", "_foreign_keys": "on"}),
			initialize: true,
		})
		if err != nil {
			return nil, err
		}
		return db, nil
	}

	db, err := open(openOptions{
		path:       path,
		dsn:        sqliteDSN(path, map[string]string{"mode": "ro", "immutable": "1", "_foreign_keys": "on"}),
		initialize: false,
		readOnly:   true,
	})
	if err != nil {
		return nil, err
	}
	if err := CheckCompatible(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func open(options openOptions) (*sql.DB, error) {
	sqlite_vec.Auto()

	if !options.readOnly {
		if err := privacy.HardenSQLiteFiles(options.path); err != nil {
			return nil, err
		}
	}

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

	if err := privacy.HardenSQLiteFiles(options.path); err != nil {
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

func sqliteDSN(path string, params map[string]string) string {
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	return (&url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: values.Encode(),
	}).String()
}

func canWriteDatabase(path string) bool {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = file.Close()

	return probeWritableDir(filepath.Dir(path)) == nil
}

func probeWritableDir(dir string) error {
	file, err := os.CreateTemp(dir, ".thr-write-probe-*")
	if err != nil {
		return err
	}
	name := file.Name()
	closeErr := file.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func IsMigrationRequired(err error) bool {
	return errors.Is(err, ErrMigrationRequired)
}

func IsDatabaseNotFound(err error) bool {
	return errors.Is(err, ErrDatabaseNotFound)
}
