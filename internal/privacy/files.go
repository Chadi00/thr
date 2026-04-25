package privacy

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	PrivateDirMode  os.FileMode = 0o700
	PrivateFileMode os.FileMode = 0o600
)

func EnsurePrivateDir(path string) error {
	if err := os.MkdirAll(path, PrivateDirMode); err != nil {
		return fmt.Errorf("create private directory %s: %w", path, err)
	}
	if err := os.Chmod(path, PrivateDirMode); err != nil {
		return fmt.Errorf("harden private directory %s: %w", path, err)
	}
	return nil
}

func HardenDirIfExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	if err := os.Chmod(path, PrivateDirMode); err != nil {
		return fmt.Errorf("harden directory %s: %w", path, err)
	}
	return nil
}

func EnsurePrivateFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, PrivateFileMode)
	if err != nil {
		return fmt.Errorf("create private file %s: %w", path, err)
	}
	closeErr := file.Close()
	if err := os.Chmod(path, PrivateFileMode); err != nil {
		return fmt.Errorf("harden private file %s: %w", path, err)
	}
	if closeErr != nil {
		return fmt.Errorf("close private file %s: %w", path, closeErr)
	}
	return nil
}

func HardenFileIfExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if err := os.Chmod(path, PrivateFileMode); err != nil {
		return fmt.Errorf("harden file %s: %w", path, err)
	}
	return nil
}

func HardenSQLiteFiles(dbPath string) error {
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := HardenFileIfExists(path); err != nil {
			return err
		}
	}
	return nil
}

func HardenTreeIfExists(root string) error {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat tree %s: %w", root, err)
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return os.Chmod(path, PrivateDirMode)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			return os.Chmod(path, PrivateFileMode)
		}
		return nil
	})
}
