package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/store"
)

type Stats struct {
	DBPath     string `json:"db_path"`
	ModelCache string `json:"model_cache"`
	Memories   int64  `json:"memories"`
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
	fmt.Fprintf(w, "%d\t%s\t%s\n", memory.ID, memory.UpdatedAt.Format(time.RFC3339), memory.Text)
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
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return strings.TrimSpace(value[:max-3]) + "..."
}

func inlineText(value string) string {
	replacer := strings.NewReplacer(
		"\r", "\\r",
		"\n", "\\n",
		"\t", "\\t",
	)
	return replacer.Replace(value)
}
