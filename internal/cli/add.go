package cli

import (
	"context"
	"fmt"

	"github.com/chadiek/thr/internal/output"
	"github.com/spf13/cobra"
)

func newAddCommand(dbPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <text>",
		Short: "Store a memory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			deps, cleanup, err := initRuntime(*dbPath, true)
			if err != nil {
				return err
			}
			defer cleanup()

			text := args[0]
			embedding, err := deps.embedder.PassageEmbed(text)
			if err != nil {
				return fmt.Errorf("embed memory text: %w", err)
			}

			memory, err := deps.repo.AddMemory(ctx, text, embedding)
			if err != nil {
				return err
			}

			output.PrintMemoryAdded(cmd.OutOrStdout(), memory)
			return nil
		},
	}

	return cmd
}
