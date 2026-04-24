package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/store"
)

func TestPrintMemoryListEscapesMultilineText(t *testing.T) {
	memory := domain.Memory{ID: 1, Text: "line one\nline two\tindent", UpdatedAt: time.Unix(0, 0).UTC()}
	buf := new(bytes.Buffer)

	PrintMemoryList(buf, []domain.Memory{memory})

	got := buf.String()
	if strings.Contains(got, "line two\tindent\n") {
		t.Fatalf("expected escaped inline output, got %q", got)
	}
	if !strings.Contains(got, `line one\nline two\tindent`) {
		t.Fatalf("expected escaped newlines and tabs, got %q", got)
	}
}

func TestPrintSearchResultsEscapesMultilineText(t *testing.T) {
	memory := domain.Memory{ID: 7, Text: "alpha\nbeta"}
	buf := new(bytes.Buffer)

	PrintSearchResults(buf, []domain.Memory{memory})

	if got := buf.String(); !strings.Contains(got, `alpha\nbeta`) {
		t.Fatalf("expected escaped multiline text, got %q", got)
	}
}

func TestPrintSemanticResultsEscapesMultilineText(t *testing.T) {
	result := store.SemanticHit{Memory: domain.Memory{ID: 3, Text: "first\nsecond"}, Distance: 0.123456}
	buf := new(bytes.Buffer)

	PrintSemanticResults(buf, []store.SemanticHit{result}, true)

	if got := buf.String(); !strings.Contains(got, `first\nsecond`) {
		t.Fatalf("expected escaped multiline text, got %q", got)
	}
}
