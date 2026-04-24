package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenExistingReturnsDatabaseNotFound(t *testing.T) {
	_, err := OpenExisting(filepath.Join(t.TempDir(), "missing.db"))
	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Fatalf("expected ErrDatabaseNotFound, got %v", err)
	}
}
