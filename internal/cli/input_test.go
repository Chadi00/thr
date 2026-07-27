package cli

import (
	"strings"
	"testing"

	"github.com/Chadi00/thr/internal/embed"
)

func TestMemoryTextCharacterLimit(t *testing.T) {
	atLimit := strings.Repeat("é", embed.MaxMemoryTextCodePoints)
	if got, err := readTextArgOrExplicitStdin(atLimit, defaultMaxMemoryBytes); err != nil || got != atLimit {
		t.Fatalf("expected %d Unicode code points to be accepted, got %q, %v", embed.MaxMemoryTextCodePoints, got, err)
	}

	if _, err := readTextArgOrExplicitStdin(atLimit+"é", defaultMaxMemoryBytes); err == nil || !strings.Contains(err.Error(), "maximum is 508 Unicode code points") {
		t.Fatalf("expected character limit error, got %v", err)
	}

	if got, err := readFromReader(strings.NewReader(atLimit), defaultMaxMemoryBytes); err != nil || got != atLimit {
		t.Fatalf("expected stdin boundary text to be accepted, got %q, %v", got, err)
	}

	if _, err := readFromReader(strings.NewReader(atLimit+"é"), defaultMaxMemoryBytes); err == nil || !strings.Contains(err.Error(), "maximum is 508 Unicode code points") {
		t.Fatalf("expected stdin character limit error, got %v", err)
	}
}

func TestMemoryTextRejectsInvalidUTF8(t *testing.T) {
	_, err := readFromReader(strings.NewReader("\xff"), defaultMaxMemoryBytes)
	if err == nil || err.Error() != "memory text must be valid UTF-8" {
		t.Fatalf("expected invalid UTF-8 error, got %v", err)
	}
}
