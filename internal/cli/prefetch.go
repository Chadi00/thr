package cli

import (
	"fmt"

	"github.com/Chadi00/thr/internal/config"
	"github.com/spf13/cobra"
)

func newPrefetchCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "prefetch",
		Short: "Download the embedding model into the local cache",
		Long: `Initializes the BGE embedding model (BAAI/bge-base-en-v1.5) in ~/.thr/models by default.
The install script runs this after building so the first add or ask is not slow.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cleanup, err := initRuntime(*dbPath, true, true)
			if err != nil {
				return err
			}
			defer cleanup()
			cfg, err := config.Load(*dbPath)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Embedding model ready (cache: %s)\n", cfg.ModelCache)
			return nil
		},
	}
}
