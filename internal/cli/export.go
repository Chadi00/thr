package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

type exportMemory struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newExportCommand(dbPath *string) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export memories as JSONL",
		Long:  "Export memories to JSONL (one memory per line) for backup and portability.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			deps, cleanup, err := initRuntime(*dbPath, false, false)
			if err != nil {
				return err
			}
			defer cleanup()

			memories, err := deps.repo.ListMemories(ctx, limit)
			if err != nil {
				return err
			}

			enc := json.NewEncoder(cmd.OutOrStdout())
			for _, memory := range memories {
				if err := enc.Encode(exportMemory{
					ID:        memory.ID,
					Text:      memory.Text,
					CreatedAt: memory.CreatedAt,
					UpdatedAt: memory.UpdatedAt,
				}); err != nil {
					return fmt.Errorf("encode exported memory %d: %w", memory.ID, err)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 1000000, "Maximum memories to export")
	return cmd
}
