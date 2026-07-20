package cli

import (
	"fmt"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

const maxSemanticDistance = 4.0

func newAskCommand(dbPath *string) *cobra.Command {
	var limit int
	var maxDistance float64
	var withDistance bool
	var scopes []string
	var allScopes bool

	cmd := &cobra.Command{
		Use:   "ask <question>",
		Short: "Retrieve semantically similar memories for a question",
		Long:  "ask performs vector retrieval over stored memories and returns the closest matches; it does not generate LLM answers.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if maxDistance <= 0 || maxDistance > maxSemanticDistance {
				return fmt.Errorf("--max-distance must be greater than 0 and at most 4")
			}

			deps, selection, cleanup, err := resolveReadRuntime(cmd, *dbPath, scopes, allScopes)
			if err != nil {
				return err
			}
			defer cleanup()
			if deps.repo == nil {
				if isJSONV2Output(cmd) {
					return encodeV2(cmd, "memory.ask", selection, map[string]any{"query": args[0], "matches": []output.SemanticMatchDTO{}}, nil)
				}
				if isJSONOutput(cmd) {
					return output.PrintSemanticResultsJSON(cmd.OutOrStdout(), nil)
				}
				output.PrintSemanticResults(cmd.OutOrStdout(), nil, withDistance)
				printHumanWarnings(cmd, selection, nil)
				return nil
			}

			health, err := deps.repo.IndexHealth(cmd.Context(), selection.IDs(), activeEmbeddingIdentity())
			if err != nil {
				return err
			}
			if health.Stale > 0 || health.MissingEmbeddings > 0 {
				return &commandError{Code: "index_stale", Message: "semantic index needs updating; run 'thr index'", SuggestedCommand: "thr index"}
			}
			if err := initEmbedder(deps, false); err != nil {
				return err
			}

			vector, err := deps.embedder.QueryEmbed(args[0])
			if err != nil {
				return err
			}
			results, err := deps.repo.SemanticSearch(cmd.Context(), vector, selection.IDs(), limit, activeEmbeddingIdentity(), maxDistance)
			if err != nil {
				return err
			}
			memories := make([]domain.Memory, len(results))
			for i, result := range results {
				memories[i] = result.Memory
			}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "memory.ask", selection, map[string]any{"query": args[0], "matches": semanticDTOs(results)}, memories)
			}
			if isJSONOutput(cmd) {
				return output.PrintSemanticResultsJSON(cmd.OutOrStdout(), results)
			}
			output.PrintSemanticResults(cmd.OutOrStdout(), results, withDistance)
			printHumanWarnings(cmd, selection, memories)
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 3, "Maximum semantic results")
	cmd.Flags().Float64Var(&maxDistance, "max-distance", store.DefaultSemanticMaxDistance, "Maximum vector distance for semantic results")
	cmd.Flags().BoolVar(&withDistance, "with-distance", false, "Print vector distance score")
	cmd.Flags().StringArrayVar(&scopes, "scope", nil, "Exact scope to search; repeat for a union")
	cmd.Flags().BoolVar(&allScopes, "all-scopes", false, "Search every registered scope")

	return cmd
}
