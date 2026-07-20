package output

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
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
	if !strings.HasPrefix(got, "ID  SCOPE") {
		t.Fatalf("expected self-describing list headers, got %q", got)
	}
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

	if got := buf.String(); strings.Join(strings.Fields(strings.SplitN(got, "\n", 2)[0]), " ") != "ID SCOPE TEXT" || !strings.Contains(got, `alpha\nbeta`) {
		t.Fatalf("expected escaped multiline text, got %q", got)
	}
}

func TestPrintSemanticResultsEscapesMultilineText(t *testing.T) {
	result := store.SemanticHit{Memory: domain.Memory{ID: 3, Text: "first\nsecond"}, Distance: 0.123456}
	buf := new(bytes.Buffer)

	PrintSemanticResults(buf, []store.SemanticHit{result}, true)

	if got := buf.String(); strings.Join(strings.Fields(strings.SplitN(got, "\n", 2)[0]), " ") != "ID DISTANCE SCOPE TEXT" || !strings.Contains(got, `first\nsecond`) {
		t.Fatalf("expected escaped multiline text, got %q", got)
	}
}

func TestPrintSemanticResultsEmptyNamesSemanticQuery(t *testing.T) {
	buf := new(bytes.Buffer)

	PrintSemanticResults(buf, nil, true)

	if got := strings.TrimSpace(buf.String()); got != "No memories matched the semantic query." {
		t.Fatalf("expected no-match semantic output, got %q", got)
	}
}

func TestPrintSemanticResultsJSONEmptyArray(t *testing.T) {
	buf := new(bytes.Buffer)

	if err := PrintSemanticResultsJSON(buf, nil); err != nil {
		t.Fatalf("print semantic json: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "[]" {
		t.Fatalf("expected empty semantic json array, got %q", got)
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
	for _, label := range []string{"ID: 2", "Scope: [user]", "Revision:", "Created:", "Updated:", "Text:"} {
		if !strings.Contains(got, label) {
			t.Fatalf("expected show output label %q in %q", label, got)
		}
	}
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

func TestLegacyJSONMemoryShapeExcludesScopeFields(t *testing.T) {
	memory := domain.Memory{
		ID: 4, Text: "memory", Scope: domain.Scope{ID: "repo:abcdefghi", Kind: domain.ScopeKindRepo, Label: "repo"},
		ScopeAssignment: domain.ScopeAssignmentExplicit, CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(2, 0).UTC(),
		ScopeUpdatedAt: time.Unix(3, 0).UTC(), Revision: 7,
	}
	buf := new(bytes.Buffer)
	if err := PrintMemoryJSON(buf, memory); err != nil {
		t.Fatalf("print json: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	want := []string{"CreatedAt", "ID", "Text", "UpdatedAt"}
	keys := make([]string, 0, len(got))
	for key := range got {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, want) {
		t.Fatalf("legacy fields = %v, want %v", keys, want)
	}

	buf.Reset()
	if err := PrintSemanticResultsJSON(buf, []store.SemanticHit{{Memory: memory, Distance: 0.25}}); err != nil {
		t.Fatalf("print semantic json: %v", err)
	}
	var hits []struct {
		Memory map[string]any
	}
	if err := json.Unmarshal(buf.Bytes(), &hits); err != nil {
		t.Fatalf("decode semantic json: %v", err)
	}
	for _, field := range []string{"Scope", "ScopeAssignment", "ScopeUpdatedAt", "Revision"} {
		if _, exists := hits[0].Memory[field]; exists {
			t.Fatalf("legacy semantic memory unexpectedly contains %q", field)
		}
	}
}

func TestHumanMemoryOutputsIncludeScopeMarkers(t *testing.T) {
	repo := domain.Memory{ID: 1, Text: "text", Scope: domain.Scope{ID: "repo:abcdefghi", Kind: domain.ScopeKindRepo, Label: "pay\nments"}}
	user := domain.Memory{ID: 2, Text: "text", Scope: domain.Scope{ID: "user", Kind: domain.ScopeKindUser, Label: "user"}}
	legacy := user
	legacy.ScopeAssignment = domain.ScopeAssignmentLegacy

	tests := []struct {
		name  string
		want  string
		print func(io.Writer)
	}{
		{name: "memory", want: `[repo:pay\nments/abcdefghi]`, print: func(w io.Writer) { PrintMemory(w, repo) }},
		{name: "list", want: "[user]", print: func(w io.Writer) { PrintMemoryList(w, []domain.Memory{user}) }},
		{name: "search", want: "[user:legacy]", print: func(w io.Writer) { PrintSearchResults(w, []domain.Memory{legacy}) }},
		{name: "semantic", want: `[repo:pay\nments/abcdefghi]`, print: func(w io.Writer) { PrintSemanticResults(w, []store.SemanticHit{{Memory: repo}}, false) }},
		{name: "add", want: "[user]", print: func(w io.Writer) { PrintMemoryAdded(w, user) }},
		{name: "forget", want: "[user:legacy]", print: func(w io.Writer) { PrintForget(w, legacy) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			test.print(buf)
			if got := buf.String(); !strings.Contains(got, test.want) {
				t.Fatalf("output %q does not contain %q", got, test.want)
			}
		})
	}
}

func TestEncodeJSONV2MemoryUsesStringID(t *testing.T) {
	memory := domain.Memory{ID: 42, Scope: domain.Scope{ID: "user", Kind: domain.ScopeKindUser}}
	buf := new(bytes.Buffer)
	if err := EncodeJSONV2(buf, Envelope{
		OK: true, Command: "memory.show", Result: NewMemoryDTO(memory),
		Context: Context{ScopeSelection: &ScopeSelection{}},
	}); err != nil {
		t.Fatalf("encode v2: %v", err)
	}

	var got struct {
		Warnings []Warning `json:"warnings"`
		Context  struct {
			ScopeSelection ScopeSelection `json:"scope_selection"`
		} `json:"context"`
		Result struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode v2: %v", err)
	}
	if got.Result.ID != "42" {
		t.Fatalf("memory id = %q, want string 42", got.Result.ID)
	}
	if got.Warnings == nil || got.Context.ScopeSelection.Requested == nil || got.Context.ScopeSelection.Resolved == nil {
		t.Fatalf("v2 nil collections encoded as null: %s", buf.String())
	}
}
