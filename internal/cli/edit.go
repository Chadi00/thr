package cli

import (
	"fmt"
	"strconv"

	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newEditCommand(dbPath *string) *cobra.Command {
	var maxBytes int64

	cmd := &cobra.Command{
		Use:   "edit <id> <text|->",
		Short: "Replace a memory",
		Long:  "Replace a memory using text. Use '-' to read replacement text from stdin explicitly.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}

			text, err := readTextArgOrExplicitStdin(args[1], maxBytes)
			if err != nil {
				return err
			}

			deps, cleanup, err := initWriteRuntimeWithEmbedder(*dbPath, false)
			if err != nil {
				return err
			}
			defer cleanup()

			embedding, err := deps.embedder.PassageEmbed(text)
			if err != nil {
				return fmt.Errorf("embed memory text: %w", err)
			}

			memory, err := deps.repo.EditMemory(cmd.Context(), id, text, embedding, activeEmbeddingIdentity())
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

	cmd.Flags().Int64Var(&maxBytes, "max-bytes", defaultMaxMemoryBytes, "Maximum memory text size in bytes")

	return cmd
}
