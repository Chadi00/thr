package cli

import (
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newSearchCommand(dbPath *string) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search memories with resilient text recall",
		Long:  "search combines indexed FTS lookup with bounded recent substring matching and fuzzy ranking for typo-tolerant recall.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, cleanup, err := initReadRuntime(*dbPath)
			if err != nil {
				if isMissingDatabase(err) {
					if isJSONOutput(cmd) {
						return output.PrintSearchResultsJSON(cmd.OutOrStdout(), nil)
					}
					output.PrintSearchResults(cmd.OutOrStdout(), nil)
					return nil
				}
				return err
			}
			defer cleanup()

			effectiveLimit := min(max(limit, 1), store.MaxSearchLimit)
			results, err := deps.repo.RecallSearch(cmd.Context(), args[0], effectiveLimit, store.DefaultRecentWindow, max(effectiveLimit*8, store.DefaultRecallCandidateMin))
			if err != nil {
				return err
			}
			if isJSONOutput(cmd) {
				return output.PrintSearchResultsJSON(cmd.OutOrStdout(), results)
			}
			output.PrintSearchResults(cmd.OutOrStdout(), results)
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "Maximum search results")

	return cmd
}
