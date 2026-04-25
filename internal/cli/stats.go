package cli

import (
	"github.com/Chadi00/thr/internal/config"
	"github.com/Chadi00/thr/internal/embed"
	"github.com/Chadi00/thr/internal/output"
	"github.com/spf13/cobra"
)

func newStatsCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show database and model cache stats",
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
			deps, cleanup, err := initReadRuntime(*dbPath)
			if err != nil {
				if !isMissingDatabase(err) {
					return err
				}
			} else {
				defer cleanup()
				health, err := deps.repo.IndexHealth(cmd.Context(), activeEmbeddingIdentity())
				if err != nil {
					return err
				}
				stats.Memories = health.Memories
				stats.IndexedMemories = health.Indexed
				stats.StaleMemories = health.Stale
				stats.MissingEmbeddings = health.MissingEmbeddings
			}

			if isJSONOutput(cmd) {
				return output.PrintStatsJSON(cmd.OutOrStdout(), stats)
			}
			output.PrintStats(cmd.OutOrStdout(), stats)
			return nil
		},
	}
}
