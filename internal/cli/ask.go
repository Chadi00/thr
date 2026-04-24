package cli

import (
	"context"
	"fmt"

	"github.com/Chadi00/thr/internal/search"
	"github.com/spf13/cobra"
)

func newAskCommand(dbPath *string) *cobra.Command {
	var limit int
	var withDistance bool

	cmd := &cobra.Command{
		Use:   "ask <question>",
		Short: "Semantically search memories and return relevant matches",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			deps, cleanup, err := initRuntime(*dbPath, true, false)
			if err != nil {
				return err
			}
			defer cleanup()

			semantic := search.NewSemanticSearcher(deps.repo, deps.embedder)
			results, err := semantic.Ask(ctx, args[0], limit)
			if err != nil {
				return err
			}

			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no matching memories")
				return nil
			}

			for _, result := range results {
				if withDistance {
					fmt.Fprintf(cmd.OutOrStdout(), "%d\t%.6f\t%s\n", result.Memory.ID, result.Distance, result.Memory.Text)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\n", result.Memory.ID, result.Memory.Text)
			}
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 3, "Maximum semantic results")
	cmd.Flags().BoolVar(&withDistance, "with-distance", false, "Print vector distance score")

	return cmd
}
