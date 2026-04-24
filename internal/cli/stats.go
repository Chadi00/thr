package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newStatsCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show database and model cache stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			deps, cleanup, err := initRuntime(*dbPath, false, false)
			if err != nil {
				return err
			}
			defer cleanup()

			count, err := deps.repo.CountMemories(ctx)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "db_path\t%s\n", deps.config.DBPath)
			fmt.Fprintf(cmd.OutOrStdout(), "model_cache\t%s\n", deps.config.ModelCache)
			fmt.Fprintf(cmd.OutOrStdout(), "memories\t%d\n", count)
			return nil
		},
	}
}
