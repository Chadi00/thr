package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Chadi00/thr/internal/privacy"
	"github.com/mattn/go-sqlite3"
)

const CurrentFormatVersion = 2

//go:embed schema.sql
var currentSchema string

var (
	ErrMigrationRequired    = errors.New("database migration required")
	ErrDatabaseIncompatible = errors.New("database format incompatible")
)

type DatabaseStatus string

const (
	DatabaseMissing           DatabaseStatus = "missing"
	DatabaseCompatible        DatabaseStatus = "compatible"
	DatabaseMigrationRequired DatabaseStatus = "migration_required"
	DatabaseIncompatible      DatabaseStatus = "incompatible"
)

type DatabaseInfo struct {
	Path   string
	Status DatabaseStatus
}

type MigrationResult struct {
	BackupPath      string
	OldFormat       int
	NewFormat       int
	Memories        int64
	Embeddings      int64
	EmbeddingModels int64
}

func InspectDatabase(path string) (DatabaseInfo, error) {
	info := DatabaseInfo{Path: path, Status: DatabaseMissing}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return info, nil
		}
		return info, fmt.Errorf("stat sqlite database: %w", err)
	}

	db, err := openReadOnly(path)
	if err != nil {
		if errors.Is(err, ErrReadOnlyWALUnavailable) {
			return info, err
		}
		return DatabaseInfo{Path: path, Status: DatabaseIncompatible}, nil
	}
	defer db.Close()
	return inspectOpenDatabase(path, db)
}

func inspectOpenDatabase(path string, db *sql.DB) (DatabaseInfo, error) {
	info := DatabaseInfo{Path: path}
	var version int
	err := db.QueryRow(`SELECT version FROM database_format WHERE singleton = 1`).Scan(&version)
	if err == nil {
		if version == CurrentFormatVersion {
			info.Status = DatabaseCompatible
			return info, nil
		}
		info.Status = DatabaseIncompatible
		return info, nil
	}

	var legacyTables int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name IN ('memories', 'memory_embeddings', 'memory_fts')
	`).Scan(&legacyTables); err == nil && legacyTables == 3 {
		info.Status = DatabaseMigrationRequired
		return info, nil
	}

	info.Status = DatabaseIncompatible
	return info, nil
}

func InspectOperationalDatabase(path string) (DatabaseInfo, error) {
	info, err := InspectDatabase(path)
	if !errors.Is(err, ErrReadOnlyWALUnavailable) {
		return info, err
	}
	db, openErr := openRaw(path, map[string]string{"mode": "rw", "_foreign_keys": "on", "_busy_timeout": "5000", "_query_only": "on"})
	if openErr != nil {
		return info, openErr
	}
	defer db.Close()
	return inspectOpenDatabase(path, db)
}

func CreateCurrentDatabase(path string) (*sql.DB, error) {
	// ponytail: keep the tiny lock file; deleting it can split concurrent waiters across different inodes.
	lock, err := os.OpenFile(path+".create.lock", os.O_CREATE|os.O_RDWR, privacy.PrivateFileMode)
	if err != nil {
		return nil, fmt.Errorf("open database creation lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return nil, fmt.Errorf("lock database creation: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck

	info, err := InspectOperationalDatabase(path)
	if err != nil {
		return nil, err
	}
	if info.Status == DatabaseCompatible {
		return openCurrentWritable(path)
	}
	if info.Status != DatabaseMissing {
		return nil, ErrDatabaseIncompatible
	}
	if err := privacy.EnsurePrivateFile(path); err != nil {
		return nil, err
	}
	db, err := openRaw(path, map[string]string{"_foreign_keys": "on", "_busy_timeout": "5000", "_txlock": "immediate"})
	if err != nil {
		return nil, err
	}
	if err := applyPragmas(db); err != nil {
		db.Close()
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		removeSQLiteFiles(path)
		return nil, fmt.Errorf("begin scoped database creation: %w", err)
	}
	if _, err := tx.Exec(currentSchema); err != nil {
		_ = tx.Rollback()
		db.Close()
		removeSQLiteFiles(path)
		return nil, fmt.Errorf("create scoped database: %w", err)
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		removeSQLiteFiles(path)
		return nil, fmt.Errorf("commit scoped database creation: %w", err)
	}
	if err := privacy.HardenSQLiteFiles(path); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func removeSQLiteFiles(path string) {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		_ = os.Remove(candidate)
	}
}

func MigratePath(ctx context.Context, path string) (MigrationResult, error) {
	info, err := InspectOperationalDatabase(path)
	if err != nil {
		return MigrationResult{}, err
	}
	switch info.Status {
	case DatabaseCompatible:
		return MigrationResult{OldFormat: CurrentFormatVersion, NewFormat: CurrentFormatVersion}, nil
	case DatabaseMissing:
		return MigrationResult{}, ErrDatabaseNotFound
	case DatabaseIncompatible:
		return MigrationResult{}, ErrDatabaseIncompatible
	}

	db, err := openRaw(path, map[string]string{"mode": "rw", "_foreign_keys": "on", "_busy_timeout": "5000"})
	if err != nil {
		return MigrationResult{}, fmt.Errorf("open legacy database for migration: %w", err)
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return MigrationResult{}, fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Close()
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()

	result := MigrationResult{OldFormat: 1, NewFormat: CurrentFormatVersion}
	var dataVersion int64
	if err := conn.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&dataVersion); err != nil {
		return MigrationResult{}, fmt.Errorf("read legacy database version: %w", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories`).Scan(&result.Memories); err != nil {
		if completed, ok := completedConcurrentMigration(path); ok {
			return completed, nil
		}
		return MigrationResult{}, fmt.Errorf("count legacy memories: %w", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_embeddings`).Scan(&result.Embeddings); err != nil {
		if completed, ok := completedConcurrentMigration(path); ok {
			return completed, nil
		}
		return MigrationResult{}, fmt.Errorf("count legacy embeddings: %w", err)
	}
	hasEmbeddingMetadata := tableExistsConn(ctx, conn, "memory_embedding_metadata")
	if hasEmbeddingMetadata {
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_embedding_metadata`).Scan(&result.EmbeddingModels); err != nil {
			if completed, ok := completedConcurrentMigration(path); ok {
				return completed, nil
			}
			return MigrationResult{}, fmt.Errorf("count legacy embedding metadata: %w", err)
		}
	}
	var sequence int64
	_ = conn.QueryRowContext(ctx, `SELECT seq FROM sqlite_sequence WHERE name = 'memories'`).Scan(&sequence)

	result.BackupPath, err = nextBackupPath(path)
	if err != nil {
		return MigrationResult{}, err
	}
	if err := backupConnection(ctx, conn, result.BackupPath); err != nil {
		_ = os.Remove(result.BackupPath)
		return MigrationResult{}, err
	}
	verifyErr := verifyLegacyBackup(result.BackupPath, result)
	if _, err := conn.ExecContext(ctx, `BEGIN EXCLUSIVE`); err != nil {
		return MigrationResult{}, fmt.Errorf("lock database for migration: %w", err)
	}
	var currentVersion int
	if err := conn.QueryRowContext(ctx, `SELECT version FROM database_format WHERE singleton = 1`).Scan(&currentVersion); err == nil && currentVersion == CurrentFormatVersion {
		_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		committed = true
		_ = os.Remove(result.BackupPath)
		return MigrationResult{OldFormat: CurrentFormatVersion, NewFormat: CurrentFormatVersion}, nil
	}
	if verifyErr != nil {
		return MigrationResult{}, verifyErr
	}
	var lockedVersion, lockedMemories, lockedEmbeddings, lockedMetadata int64
	if err := conn.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&lockedVersion); err != nil {
		return MigrationResult{}, fmt.Errorf("recheck legacy database version: %w", err)
	}
	_ = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memories`).Scan(&lockedMemories)
	_ = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_embeddings`).Scan(&lockedEmbeddings)
	if hasEmbeddingMetadata {
		_ = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_embedding_metadata`).Scan(&lockedMetadata)
	}
	if lockedVersion != dataVersion || lockedMemories != result.Memories || lockedEmbeddings != result.Embeddings || lockedMetadata != result.EmbeddingModels {
		_ = os.Remove(result.BackupPath)
		return MigrationResult{}, errors.New("database changed while migration backup was created; retry migration")
	}

	if _, err := conn.ExecContext(ctx, currentSchema); err != nil {
		return MigrationResult{}, fmt.Errorf("create scoped schema: %w", err)
	}
	now := time.Now().UTC().UnixMilli()
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO scoped_memories(
			id, text, scope_id, scope_assignment, created_at, updated_at, scope_updated_at, revision
		)
		SELECT id, text, 'user', 'legacy_default', created_at, updated_at, ?, 1 FROM memories
	`, now); err != nil {
		return MigrationResult{}, fmt.Errorf("migrate memories: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO scoped_memory_embeddings(rowid, scope_id, embedding)
		SELECT rowid, 'user', embedding FROM memory_embeddings
	`); err != nil {
		return MigrationResult{}, fmt.Errorf("migrate embeddings: %w", err)
	}
	if hasEmbeddingMetadata {
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO scoped_memory_embedding_metadata(
				memory_id, model_id, model_revision, manifest_sha256, dimension, indexed_at
			)
			SELECT memory_id, model_id, model_revision, manifest_sha256, dimension, indexed_at
			FROM memory_embedding_metadata
		`); err != nil {
			return MigrationResult{}, fmt.Errorf("migrate embedding metadata: %w", err)
		}
	}
	if sequence > 0 {
		if _, err := conn.ExecContext(ctx, `UPDATE sqlite_sequence SET seq = ? WHERE name = 'scoped_memories'`, sequence); err != nil {
			return MigrationResult{}, fmt.Errorf("preserve memory id sequence: %w", err)
		}
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO scoped_memory_fts(scoped_memory_fts) VALUES('integrity-check')`); err != nil {
		return MigrationResult{}, fmt.Errorf("validate migrated full-text index: %w", err)
	}

	var memories, embeddings, metadata int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM scoped_memories`).Scan(&memories); err != nil {
		return MigrationResult{}, fmt.Errorf("validate migrated memory count: %w", err)
	}
	if memories != result.Memories {
		return MigrationResult{}, fmt.Errorf("validate migrated memory count: got %d want %d", memories, result.Memories)
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM scoped_memory_embeddings`).Scan(&embeddings); err != nil {
		return MigrationResult{}, fmt.Errorf("validate migrated embedding count: %w", err)
	}
	if embeddings != result.Embeddings {
		return MigrationResult{}, fmt.Errorf("validate migrated embedding count: got %d want %d", embeddings, result.Embeddings)
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM scoped_memory_embedding_metadata`).Scan(&metadata); err != nil {
		return MigrationResult{}, fmt.Errorf("validate migrated metadata count: %w", err)
	}
	if metadata != result.EmbeddingModels {
		return MigrationResult{}, fmt.Errorf("validate migrated metadata count: got %d want %d", metadata, result.EmbeddingModels)
	}

	if _, err := conn.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS memories_ai;
		DROP TRIGGER IF EXISTS memories_ad;
		DROP TRIGGER IF EXISTS memories_au;
		DROP TABLE memory_fts;
		DROP TABLE memory_embeddings;
		DROP TABLE IF EXISTS memory_embedding_metadata;
		DROP TABLE memories;
		DROP TABLE IF EXISTS schema_version;
	`); err != nil {
		return MigrationResult{}, fmt.Errorf("remove legacy schema: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return MigrationResult{}, fmt.Errorf("commit scope migration: %w", err)
	}
	committed = true
	if err := privacy.HardenSQLiteFiles(path); err != nil {
		return MigrationResult{}, err
	}
	return result, nil
}

func completedConcurrentMigration(path string) (MigrationResult, bool) {
	info, err := InspectOperationalDatabase(path)
	if err == nil && info.Status == DatabaseCompatible {
		return MigrationResult{OldFormat: CurrentFormatVersion, NewFormat: CurrentFormatVersion}, true
	}
	return MigrationResult{}, false
}

func nextBackupPath(path string) (string, error) {
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".backup-"+stamp+"-*")
	if err != nil {
		return "", fmt.Errorf("create migration backup: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return file.Name(), nil
}

func backupConnection(ctx context.Context, source *sql.Conn, path string) error {
	destination, err := openRaw(path, map[string]string{"mode": "rw", "_foreign_keys": "on"})
	if err != nil {
		return fmt.Errorf("open migration backup: %w", err)
	}
	defer destination.Close()
	destinationConn, err := destination.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire backup connection: %w", err)
	}
	defer destinationConn.Close()

	return destinationConn.Raw(func(destinationDriver any) error {
		dest, ok := destinationDriver.(*sqlite3.SQLiteConn)
		if !ok {
			return errors.New("unexpected SQLite backup driver")
		}
		return source.Raw(func(sourceDriver any) error {
			src, ok := sourceDriver.(*sqlite3.SQLiteConn)
			if !ok {
				return errors.New("unexpected SQLite source driver")
			}
			backup, err := dest.Backup("main", src, "main")
			if err != nil {
				return fmt.Errorf("start SQLite backup: %w", err)
			}
			for {
				done, err := backup.Step(-1)
				if err != nil {
					_ = backup.Close()
					return fmt.Errorf("copy SQLite backup: %w", err)
				}
				if done {
					break
				}
				select {
				case <-ctx.Done():
					_ = backup.Close()
					return ctx.Err()
				case <-time.After(10 * time.Millisecond):
				}
			}
			if err := backup.Close(); err != nil {
				return fmt.Errorf("finish SQLite backup: %w", err)
			}
			return nil
		})
	})
}

func verifyLegacyBackup(path string, expected MigrationResult) error {
	db, err := openRaw(path, map[string]string{"mode": "ro", "immutable": "1", "_query_only": "on"})
	if err != nil {
		return fmt.Errorf("open migration backup for verification: %w", err)
	}
	defer db.Close()
	var memories, embeddings, metadata int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&memories); err != nil {
		return fmt.Errorf("verify backup memories: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM memory_embeddings`).Scan(&embeddings); err != nil {
		return fmt.Errorf("verify backup embeddings: %w", err)
	}
	if tableExistsDB(db, "memory_embedding_metadata") {
		if err := db.QueryRow(`SELECT COUNT(*) FROM memory_embedding_metadata`).Scan(&metadata); err != nil {
			return fmt.Errorf("verify backup embedding metadata: %w", err)
		}
	}
	if memories != expected.Memories || embeddings != expected.Embeddings || metadata != expected.EmbeddingModels {
		return fmt.Errorf("verify migration backup counts: memories=%d/%d embeddings=%d/%d metadata=%d/%d",
			memories, expected.Memories, embeddings, expected.Embeddings, metadata, expected.EmbeddingModels)
	}
	return nil
}

func tableExistsConn(ctx context.Context, conn *sql.Conn, name string) bool {
	var exists int
	return conn.QueryRowContext(ctx, `SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&exists) == nil
}

func tableExistsDB(db *sql.DB, name string) bool {
	var exists int
	return db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&exists) == nil
}
