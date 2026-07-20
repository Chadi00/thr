package cli

import (
	"fmt"

	"github.com/Chadi00/thr/internal/config"
	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/embed"
	"github.com/Chadi00/thr/internal/output"
	"github.com/spf13/cobra"
)

func newStatsCommand(dbPath *string) *cobra.Command {
	var scopes []string
	var allScopes bool
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show database and model cache stats",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*dbPath)
			if err != nil {
				return err
			}

			model := embed.ActiveModelStatus(cfg.ModelCache)
			stats := output.Stats{
				DBPath:              cfg.DBPath,
				ModelCache:          cfg.ModelCache,
				ModelID:             model.ModelID,
				ModelRevision:       model.ModelRevision,
				ModelManifestSHA256: model.ManifestSHA256,
				ModelVerified:       model.Verified,
			}
			deps, selection, cleanup, err := resolveReadRuntime(cmd, *dbPath, scopes, allScopes || len(scopes) == 0)
			if err != nil {
				return err
			}
			defer cleanup()
			selection.Warnings = managedSkillWarnings()
			if deps.repo == nil && len(scopes) == 0 && !allScopes {
				selection.Scopes = []domain.Scope{{ID: "user", Kind: domain.ScopeKindUser, Label: "user"}}
			}
			perScope := make([]map[string]any, 0, len(selection.Scopes))
			if deps.repo != nil {
				health, err := deps.repo.IndexHealth(cmd.Context(), selection.IDs(), activeEmbeddingIdentity())
				if err != nil {
					return err
				}
				stats.Memories = health.Memories
				stats.IndexedMemories = health.Indexed
				stats.StaleMemories = health.Stale
				stats.MissingEmbeddings = health.MissingEmbeddings
				for _, scope := range selection.Scopes {
					scopeHealth, err := deps.repo.IndexHealth(cmd.Context(), []string{scope.ID}, activeEmbeddingIdentity())
					if err != nil {
						return err
					}
					perScope = append(perScope, map[string]any{"scope": output.NewScopeDTO(scope), "memories": scopeHealth.Memories, "indexed": scopeHealth.Indexed, "stale": scopeHealth.Stale, "missing": scopeHealth.MissingEmbeddings})
				}
			} else if len(selection.Scopes) == 1 {
				perScope = append(perScope, map[string]any{"scope": output.NewScopeDTO(selection.Scopes[0]), "memories": 0, "indexed": 0, "stale": 0, "missing": 0})
			}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "memory.stats", selection, map[string]any{"stats": stats, "scopes": perScope}, nil)
			}

			if isJSONOutput(cmd) {
				return output.PrintStatsJSON(cmd.OutOrStdout(), stats)
			}
			output.PrintStats(cmd.OutOrStdout(), stats)
			for _, row := range perScope {
				scope := row["scope"].(output.ScopeDTO)
				fmt.Fprintf(cmd.OutOrStdout(), "scope\t%s\tmemories=%v\tindexed=%v\tstale=%v\tmissing=%v\n", output.SanitizeInline(scope.ID), row["memories"], row["indexed"], row["stale"], row["missing"])
			}
			for _, warning := range selection.Warnings {
				fmt.Fprintf(cmd.OutOrStdout(), "warning: %s; run %s\n", warning.Message, warning.Details["suggested_command"])
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&scopes, "scope", nil, "Exact scope to inspect; repeat for a union")
	cmd.Flags().BoolVar(&allScopes, "all-scopes", false, "Inspect every registered scope")
	return cmd
}
