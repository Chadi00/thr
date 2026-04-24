package output

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Chadi00/thr/internal/domain"
)

func PrintMemoryAdded(w io.Writer, memory domain.Memory) {
	fmt.Fprintf(w, "stored memory %d\n", memory.ID)
}

func PrintMemoryList(w io.Writer, memories []domain.Memory) {
	if len(memories) == 0 {
		fmt.Fprintln(w, "no memories stored")
		return
	}

	for _, memory := range memories {
		fmt.Fprintf(w, "%d\t%s\t%s\n", memory.ID, memory.UpdatedAt.Format(time.RFC3339), truncate(memory.Text, 120))
	}
}

func PrintSearchResults(w io.Writer, memories []domain.Memory) {
	if len(memories) == 0 {
		fmt.Fprintln(w, "no matching memories")
		return
	}

	for _, memory := range memories {
		fmt.Fprintf(w, "%d\t%s\n", memory.ID, memory.Text)
	}
}

func PrintForget(w io.Writer, id int64) {
	fmt.Fprintf(w, "forgot memory %d\n", id)
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
