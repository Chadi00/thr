package cli

import (
	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newSearchCommand(dbPath *string) *cobra.Command {
	var limit int
	var scopes []string
	var allScopes bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search memories with resilient text recall",
		Long:  "search combines indexed FTS lookup with bounded recent substring matching and fuzzy ranking for typo-tolerant recall.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, selection, cleanup, err := resolveReadRuntime(cmd, *dbPath, scopes, allScopes)
			if err != nil {
				return err
			}
			defer cleanup()

			effectiveLimit := min(max(limit, 1), store.MaxSearchLimit)
			var results []domain.Memory
			if deps.repo != nil {
				results, err = deps.repo.RecallSearch(cmd.Context(), args[0], selection.IDs(), effectiveLimit, store.DefaultRecentWindow, max(effectiveLimit*8, store.DefaultRecallCandidateMin))
				if err != nil {
					return err
				}
			}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "memory.search", selection, map[string]any{"query": args[0], "matches": memoryDTOs(results)}, results)
			}
			if isJSONOutput(cmd) {
				return output.PrintSearchResultsJSON(cmd.OutOrStdout(), results)
			}
			output.PrintSearchResults(cmd.OutOrStdout(), results)
			printHumanWarnings(cmd, selection, results)
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "Maximum search results")
	cmd.Flags().StringArrayVar(&scopes, "scope", nil, "Exact scope to search; repeat for a union")
	cmd.Flags().BoolVar(&allScopes, "all-scopes", false, "Search every registered scope")

	return cmd
}
