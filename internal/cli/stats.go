package cli

import (
	"github.com/Chadi00/thr/internal/config"
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

			stats := output.Stats{DBPath: cfg.DBPath, ModelCache: cfg.ModelCache}
			deps, cleanup, err := initReadRuntime(*dbPath)
			if err != nil {
				if !isMissingDatabase(err) {
					return err
				}
			} else {
				defer cleanup()
				count, err := deps.repo.CountMemories(cmd.Context())
				if err != nil {
					return err
				}
				stats.Memories = count
			}

			if isJSONOutput(cmd) {
				return output.PrintStatsJSON(cmd.OutOrStdout(), stats)
			}
			output.PrintStats(cmd.OutOrStdout(), stats)
			return nil
		},
	}
}
