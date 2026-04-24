package cli

import (
	"context"

	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/search"
	"github.com/spf13/cobra"
)

func newSearchCommand(dbPath *string) *cobra.Command {
	var limit int
	var substring bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Keyword search over memory text (FTS5)",
		Long:  "Keyword search uses SQLite FTS5 token matching (porter unicode61), so terms match words/tokens rather than arbitrary substrings unless --substring is used.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			deps, cleanup, err := initRuntime(*dbPath, false, false)
			if err != nil {
				return err
			}
			defer cleanup()

			if substring {
				results, err := deps.repo.SubstringSearch(ctx, args[0], limit)
				if err != nil {
					return err
				}
				if isJSONOutput(cmd) {
					return output.PrintSearchResultsJSON(cmd.OutOrStdout(), results)
				}
				output.PrintSearchResults(cmd.OutOrStdout(), results)
				return nil
			}

			keyword := search.NewKeywordSearcher(deps.repo)
			results, err := keyword.Search(ctx, args[0], limit)
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

	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "Maximum keyword matches")
	cmd.Flags().BoolVar(&substring, "substring", false, "Use substring matching via SQL LIKE instead of token-based FTS")

	return cmd
}
