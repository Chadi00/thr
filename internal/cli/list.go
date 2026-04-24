package cli

import (
	"context"

	"github.com/Chadi00/thr/internal/output"
	"github.com/spf13/cobra"
)

func newListCommand(dbPath *string) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List stored memories",
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

			if isJSONOutput(cmd) {
				return output.PrintMemoryListJSON(cmd.OutOrStdout(), memories)
			}
			output.PrintMemoryList(cmd.OutOrStdout(), memories)
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 100, "Maximum memories to list")

	return cmd
}
