package cli

import (
	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/output"
	"github.com/spf13/cobra"
)

func newListCommand(dbPath *string) *cobra.Command {
	var limit int
	var scopes []string
	var allScopes bool
	var legacyOnly bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List stored memories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, selection, cleanup, err := resolveReadRuntime(cmd, *dbPath, scopes, allScopes)
			if err != nil {
				return err
			}
			defer cleanup()

			var memories []domain.Memory
			if deps.repo != nil {
				memories, err = deps.repo.ListMemories(cmd.Context(), selection.IDs(), limit, legacyOnly)
				if err != nil {
					return err
				}
			}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "memory.list", selection, map[string]any{"memories": memoryDTOs(memories)}, memories)
			}
			if isJSONOutput(cmd) {
				return output.PrintMemoryListJSON(cmd.OutOrStdout(), memories)
			}
			output.PrintMemoryList(cmd.OutOrStdout(), memories)
			printHumanWarnings(cmd, selection, memories)
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 100, "Maximum memories to list")
	cmd.Flags().IntVar(&limit, "last", 100, "Alias for --limit; list the last N memories saved")
	cmd.Flags().StringArrayVar(&scopes, "scope", nil, "Exact scope to list; repeat for a union")
	cmd.Flags().BoolVar(&allScopes, "all-scopes", false, "List every registered scope")
	cmd.Flags().BoolVar(&legacyOnly, "legacy", false, "List only migrated legacy assignments")

	return cmd
}
