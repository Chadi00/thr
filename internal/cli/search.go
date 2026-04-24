package cli

import (
	"context"

	"github.com/chadiek/thr/internal/output"
	"github.com/chadiek/thr/internal/search"
	"github.com/spf13/cobra"
)

func newSearchCommand(dbPath *string) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Keyword search over memory text",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			deps, cleanup, err := initRuntime(*dbPath, false)
			if err != nil {
				return err
			}
			defer cleanup()

			keyword := search.NewKeywordSearcher(deps.repo)
			results, err := keyword.Search(ctx, args[0], limit)
			if err != nil {
				return err
			}

			output.PrintSearchResults(cmd.OutOrStdout(), results)
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "Maximum keyword matches")

	return cmd
}
