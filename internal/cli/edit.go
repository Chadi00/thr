package cli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newEditCommand(dbPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <id> <text|->",
		Short: "Replace a memory",
		Long:  "Replace a memory using text. Use '-' to read replacement text from stdin explicitly.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			deps, cleanup, err := initRuntime(*dbPath, true, false)
			if err != nil {
				return err
			}
			defer cleanup()

			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}

			text, err := readTextArgOrExplicitStdin(args[1])
			if err != nil {
				return err
			}
			embedding, err := deps.embedder.PassageEmbed(text)
			if err != nil {
				return fmt.Errorf("embed memory text: %w", err)
			}

			memory, err := deps.repo.EditMemory(ctx, id, text, embedding)
			if err != nil {
				if err == store.ErrMemoryNotFound {
					return fmt.Errorf("memory %d not found", id)
				}
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "updated memory %d\n", memory.ID)
			return nil
		},
	}

	return cmd
}
