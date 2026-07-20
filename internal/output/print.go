package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/store"
)

type Stats struct {
	DBPath              string `json:"db_path"`
	ModelCache          string `json:"model_cache"`
	Memories            int64  `json:"memories"`
	ModelID             string `json:"model_id"`
	ModelRevision       string `json:"model_revision"`
	ModelManifestSHA256 string `json:"model_manifest_sha256"`
	ModelVerified       bool   `json:"model_verified"`
	IndexedMemories     int64  `json:"indexed_memories"`
	StaleMemories       int64  `json:"stale_memories"`
	MissingEmbeddings   int64  `json:"missing_embeddings"`
}

type legacyMemory struct {
	ID        int64
	Text      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type legacySemanticHit struct {
	Memory   legacyMemory
	Distance float64
}

func PrintMemoryAdded(w io.Writer, memory domain.Memory) {
	fmt.Fprintf(w, "Stored memory %d in %s.\n", memory.ID, memoryScopeMarker(memory))
}

func PrintMemoryList(w io.Writer, memories []domain.Memory) {
	if len(memories) == 0 {
		fmt.Fprintln(w, "No memories found in the selected scopes.")
		return
	}

	table := NewTable(w)
	fmt.Fprintln(table, "ID\tSCOPE\tUPDATED\tTEXT")
	for _, memory := range memories {
		fmt.Fprintf(table, "%d\t%s\t%s\t%s\n", memory.ID, memoryScopeMarker(memory), memory.UpdatedAt.Format(time.RFC3339), truncate(inlineText(memory.Text), 120))
	}
	_ = table.Flush()
}

func PrintMemoryListJSON(w io.Writer, memories []domain.Memory) error {
	return encodeJSON(w, legacyMemories(memories))
}

func PrintMemory(w io.Writer, memory domain.Memory) {
	fmt.Fprintf(w, "ID: %d\n", memory.ID)
	fmt.Fprintf(w, "Scope: %s\n", memoryScopeMarker(memory))
	fmt.Fprintf(w, "Revision: %d\n", memory.Revision)
	fmt.Fprintf(w, "Created: %s\n", memory.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "Updated: %s\n", memory.UpdatedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "Text:\n%s\n", sanitizeText(memory.Text, true))
}

func PrintMemoryJSON(w io.Writer, memory domain.Memory) error {
	return encodeJSON(w, newLegacyMemory(memory))
}

func PrintSearchResults(w io.Writer, memories []domain.Memory) {
	if len(memories) == 0 {
		fmt.Fprintln(w, "No memories matched the text query.")
		return
	}

	table := NewTable(w)
	fmt.Fprintln(table, "ID\tSCOPE\tTEXT")
	for _, memory := range memories {
		fmt.Fprintf(table, "%d\t%s\t%s\n", memory.ID, memoryScopeMarker(memory), inlineText(memory.Text))
	}
	_ = table.Flush()
}

func PrintSearchResultsJSON(w io.Writer, memories []domain.Memory) error {
	return encodeJSON(w, legacyMemories(memories))
}

func PrintForget(w io.Writer, memory domain.Memory) {
	fmt.Fprintf(w, "Deleted memory %d from %s.\n", memory.ID, memoryScopeMarker(memory))
}

func PrintSemanticResults(w io.Writer, results []store.SemanticHit, withDistance bool) {
	if len(results) == 0 {
		fmt.Fprintln(w, "No memories matched the semantic query.")
		return
	}
	table := NewTable(w)
	if withDistance {
		fmt.Fprintln(table, "ID\tDISTANCE\tSCOPE\tTEXT")
	} else {
		fmt.Fprintln(table, "ID\tSCOPE\tTEXT")
	}
	for _, result := range results {
		if withDistance {
			fmt.Fprintf(table, "%d\t%.6f\t%s\t%s\n", result.Memory.ID, result.Distance, memoryScopeMarker(result.Memory), inlineText(result.Memory.Text))
			continue
		}
		fmt.Fprintf(table, "%d\t%s\t%s\n", result.Memory.ID, memoryScopeMarker(result.Memory), inlineText(result.Memory.Text))
	}
	_ = table.Flush()
}

func PrintSemanticResultsJSON(w io.Writer, results []store.SemanticHit) error {
	hits := make([]legacySemanticHit, len(results))
	for i, result := range results {
		hits[i] = legacySemanticHit{Memory: newLegacyMemory(result.Memory), Distance: result.Distance}
	}
	return encodeJSON(w, hits)
}

func PrintStats(w io.Writer, stats Stats) {
	fmt.Fprintln(w, "Database")
	fmt.Fprintf(w, "  Path: %s\n", inlineText(stats.DBPath))
	fmt.Fprintf(w, "  Memories: %d\n\n", stats.Memories)
	fmt.Fprintln(w, "Semantic index")
	fmt.Fprintf(w, "  Indexed: %d\n", stats.IndexedMemories)
	fmt.Fprintf(w, "  Stale: %d\n", stats.StaleMemories)
	fmt.Fprintf(w, "  Missing: %d\n\n", stats.MissingEmbeddings)
	fmt.Fprintln(w, "Embedding model")
	fmt.Fprintf(w, "  ID: %s\n", inlineText(stats.ModelID))
	fmt.Fprintf(w, "  Revision: %s\n", inlineText(stats.ModelRevision))
	fmt.Fprintf(w, "  Manifest SHA-256: %s\n", inlineText(stats.ModelManifestSHA256))
	fmt.Fprintf(w, "  Verified: %t\n", stats.ModelVerified)
	fmt.Fprintf(w, "  Cache: %s\n", inlineText(stats.ModelCache))
}

func PrintStatsJSON(w io.Writer, stats Stats) error {
	return encodeJSON(w, stats)
}

func encodeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func newLegacyMemory(memory domain.Memory) legacyMemory {
	return legacyMemory{
		ID: memory.ID, Text: memory.Text,
		CreatedAt: memory.CreatedAt, UpdatedAt: memory.UpdatedAt,
	}
}

func legacyMemories(memories []domain.Memory) []legacyMemory {
	result := make([]legacyMemory, len(memories))
	for i, memory := range memories {
		result[i] = newLegacyMemory(memory)
	}
	return result
}

func ScopeMarker(scope domain.Scope, assignment domain.ScopeAssignment) string {
	if assignment == domain.ScopeAssignmentLegacy {
		return "[user:legacy]"
	}
	if scope.Kind != domain.ScopeKindRepo && !strings.HasPrefix(scope.ID, "repo:") {
		return "[user]"
	}

	// The full payload is the shortest implementation that is always unambiguous.
	payload := strings.TrimPrefix(scope.ID, "repo:")
	return fmt.Sprintf("[repo:%s/%s]", inlineText(scope.Label), inlineText(payload))
}

func NewTable(w io.Writer) *tabwriter.Writer { return tabwriter.NewWriter(w, 0, 4, 2, ' ', 0) }

func SanitizeInline(value string) string { return inlineText(value) }

func memoryScopeMarker(memory domain.Memory) string {
	return ScopeMarker(memory.Scope, memory.ScopeAssignment)
}

func truncate(value string, max int) string {
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	if max <= 3 {
		return string([]rune(value)[:max])
	}
	return strings.TrimSpace(string([]rune(value)[:max-3])) + "..."
}

func inlineText(value string) string {
	return sanitizeText(value, false)
}

func sanitizeText(value string, allowNewline bool) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r == '\n' && allowNewline:
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r >= 0x80 && r <= 0x9f:
			fmt.Fprintf(&b, `\u%04x`, r)
		case unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Cc, r):
			if r <= 0xffff {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				fmt.Fprintf(&b, `\U%08x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
