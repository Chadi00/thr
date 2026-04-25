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

func TestPrintHumanOutputEscapesTerminalControls(t *testing.T) {
	memory := domain.Memory{ID: 9, Text: "safe\x1b[2J\a\b\u202eend", UpdatedAt: time.Unix(0, 0).UTC()}
	buf := new(bytes.Buffer)

	PrintSearchResults(buf, []domain.Memory{memory})

	got := buf.String()
	for _, raw := range []string{"\x1b", "\a", "\b", "\u202e"} {
		if strings.Contains(got, raw) {
			t.Fatalf("expected raw control %q to be escaped in %q", raw, got)
		}
	}
	for _, escaped := range []string{`\x1b`, `\x07`, `\x08`, `\u202e`} {
		if !strings.Contains(got, escaped) {
			t.Fatalf("expected escaped control %q in %q", escaped, got)
		}
	}
}

func TestPrintMemoryPreservesNewlinesButEscapesControls(t *testing.T) {
	memory := domain.Memory{ID: 2, Text: "line one\nline two\x1b]52;c;bad\a", UpdatedAt: time.Unix(0, 0).UTC()}
	buf := new(bytes.Buffer)

	PrintMemory(buf, memory)

	got := buf.String()
	if !strings.Contains(got, "line one\nline two") {
		t.Fatalf("expected show output to preserve memory newlines, got %q", got)
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\a") {
		t.Fatalf("expected terminal controls to be escaped, got %q", got)
	}
	if !strings.Contains(got, `\x1b]52;c;bad\x07`) {
		t.Fatalf("expected OSC controls to be escaped, got %q", got)
	}
}

func TestPrintMemoryJSONDoesNotSanitizeText(t *testing.T) {
	memory := domain.Memory{ID: 4, Text: "raw\x1b[2J", UpdatedAt: time.Unix(0, 0).UTC()}
	buf := new(bytes.Buffer)

	if err := PrintMemoryJSON(buf, memory); err != nil {
		t.Fatalf("print json: %v", err)
	}
	if !strings.Contains(buf.String(), `raw\u001b[2J`) {
		t.Fatalf("expected JSON encoding to preserve raw text semantics, got %q", buf.String())
	}
}
