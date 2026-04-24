package cli

import (
	"github.com/Chadi00/thr/internal/output"
	"github.com/spf13/cobra"
)

func newAskCommand(dbPath *string) *cobra.Command {
	var limit int
	var withDistance bool

	cmd := &cobra.Command{
		Use:   "ask <question>",
		Short: "Retrieve semantically similar memories for a question",
		Long:  "ask performs vector retrieval over stored memories and returns the closest matches; it does not generate LLM answers.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, cleanup, err := initReadRuntimeWithEmbedder(*dbPath, false)
			if err != nil {
				if isMissingDatabase(err) {
					if isJSONOutput(cmd) {
						return output.PrintSemanticResultsJSON(cmd.OutOrStdout(), nil)
					}
					output.PrintSemanticResults(cmd.OutOrStdout(), nil, withDistance)
					return nil
				}
				return err
			}
			defer cleanup()

			vector, err := deps.embedder.QueryEmbed(args[0])
			if err != nil {
				return err
			}
			results, err := deps.repo.SemanticSearch(cmd.Context(), vector, limit)
			if err != nil {
				return err
			}

			if isJSONOutput(cmd) {
				return output.PrintSemanticResultsJSON(cmd.OutOrStdout(), results)
			}
			output.PrintSemanticResults(cmd.OutOrStdout(), results, withDistance)
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 3, "Maximum semantic results")
	cmd.Flags().BoolVar(&withDistance, "with-distance", false, "Print vector distance score")

	return cmd
}
