package cli

import (
	"github.com/Chadi00/thr/internal/output"
	"github.com/spf13/cobra"
)

func newListCommand(dbPath *string) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List stored memories",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, cleanup, err := initReadRuntime(*dbPath)
			if err != nil {
				if isMissingDatabase(err) {
					if isJSONOutput(cmd) {
						return output.PrintMemoryListJSON(cmd.OutOrStdout(), nil)
					}
					output.PrintMemoryList(cmd.OutOrStdout(), nil)
					return nil
				}
				return err
			}
			defer cleanup()

			memories, err := deps.repo.ListMemories(cmd.Context(), limit)
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
	cmd.Flags().IntVar(&limit, "last", 100, "Alias for --limit; list the last N memories saved")

	return cmd
}
