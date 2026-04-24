package cli

import (
	"fmt"

	"github.com/Chadi00/thr/internal/output"
	"github.com/spf13/cobra"
)

func newAddCommand(dbPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <text|->",
		Short: "Store a memory",
		Long:  "Add a memory from text. Use '-' to read from stdin explicitly.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, cleanup, err := initWriteRuntimeWithEmbedder(*dbPath, false)
			if err != nil {
				return err
			}
			defer cleanup()

			text, err := readTextArgOrExplicitStdin(args[0])
			if err != nil {
				return err
			}
			embedding, err := deps.embedder.PassageEmbed(text)
			if err != nil {
				return fmt.Errorf("embed memory text: %w", err)
			}

			memory, err := deps.repo.AddMemory(cmd.Context(), text, embedding)
			if err != nil {
				return err
			}

			output.PrintMemoryAdded(cmd.OutOrStdout(), memory)
			return nil
		},
	}

	return cmd
}
