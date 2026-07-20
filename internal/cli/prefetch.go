package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPrefetchCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "prefetch",
		Short: "Prepare the bundled embedding model in the local cache",
		Long: `Initializes the bundled BGE embedding model (BAAI/bge-base-en-v1.5) in ~/.thr/models by default.
The install script runs this after building so the first add or ask is not slow.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, cleanup, err := initPrefetchRuntime(*dbPath, !isJSONV2Output(cmd))
			if err != nil {
				return err
			}
			defer cleanup()
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "model.prefetch", independentSelection(cmd), map[string]any{"model_cache": deps.config.ModelCache, "ready": true}, nil)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Embedding model ready (cache: %s)\n", deps.config.ModelCache)
			return nil
		},
	}
}
