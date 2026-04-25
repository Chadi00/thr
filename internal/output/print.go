package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
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

func PrintMemoryAdded(w io.Writer, memory domain.Memory) {
	fmt.Fprintf(w, "stored memory %d\n", memory.ID)
}

func PrintMemoryList(w io.Writer, memories []domain.Memory) {
	if len(memories) == 0 {
		fmt.Fprintln(w, "no memories stored")
		return
	}

	for _, memory := range memories {
		fmt.Fprintf(w, "%d\t%s\t%s\n", memory.ID, memory.UpdatedAt.Format(time.RFC3339), truncate(inlineText(memory.Text), 120))
	}
}

func PrintMemoryListJSON(w io.Writer, memories []domain.Memory) error {
	if memories == nil {
		memories = []domain.Memory{}
	}
	return encodeJSON(w, memories)
}

func PrintMemory(w io.Writer, memory domain.Memory) {
	fmt.Fprintf(w, "%d\t%s\t%s\n", memory.ID, memory.UpdatedAt.Format(time.RFC3339), sanitizeText(memory.Text, true))
}

func PrintMemoryJSON(w io.Writer, memory domain.Memory) error {
	return encodeJSON(w, memory)
}

func PrintSearchResults(w io.Writer, memories []domain.Memory) {
	if len(memories) == 0 {
		fmt.Fprintln(w, "no matching memories")
		return
	}

	for _, memory := range memories {
		fmt.Fprintf(w, "%d\t%s\n", memory.ID, inlineText(memory.Text))
	}
}

func PrintSearchResultsJSON(w io.Writer, memories []domain.Memory) error {
	if memories == nil {
		memories = []domain.Memory{}
	}
	return encodeJSON(w, memories)
}

func PrintForget(w io.Writer, id int64) {
	fmt.Fprintf(w, "forgot memory %d\n", id)
}

func PrintSemanticResults(w io.Writer, results []store.SemanticHit, withDistance bool) {
	if len(results) == 0 {
		fmt.Fprintln(w, "no matching memories")
		return
	}
	for _, result := range results {
		if withDistance {
			fmt.Fprintf(w, "%d\t%.6f\t%s\n", result.Memory.ID, result.Distance, inlineText(result.Memory.Text))
			continue
		}
		fmt.Fprintf(w, "%d\t%s\n", result.Memory.ID, inlineText(result.Memory.Text))
	}
}

func PrintSemanticResultsJSON(w io.Writer, results []store.SemanticHit) error {
	if results == nil {
		results = []store.SemanticHit{}
	}
	return encodeJSON(w, results)
}

func PrintStats(w io.Writer, stats Stats) {
	fmt.Fprintf(w, "db_path\t%s\n", stats.DBPath)
	fmt.Fprintf(w, "model_cache\t%s\n", stats.ModelCache)
	fmt.Fprintf(w, "memories\t%d\n", stats.Memories)
	fmt.Fprintf(w, "model_id\t%s\n", stats.ModelID)
	fmt.Fprintf(w, "model_revision\t%s\n", stats.ModelRevision)
	fmt.Fprintf(w, "model_manifest_sha256\t%s\n", stats.ModelManifestSHA256)
	fmt.Fprintf(w, "model_verified\t%t\n", stats.ModelVerified)
	fmt.Fprintf(w, "indexed_memories\t%d\n", stats.IndexedMemories)
	fmt.Fprintf(w, "stale_memories\t%d\n", stats.StaleMemories)
	fmt.Fprintf(w, "missing_embeddings\t%d\n", stats.MissingEmbeddings)
}

func PrintStatsJSON(w io.Writer, stats Stats) error {
	return encodeJSON(w, stats)
}

func encodeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
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
