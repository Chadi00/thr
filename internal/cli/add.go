package cli

import (
	"context"
	"fmt"

	"github.com/Chadi00/thr/internal/output"
	"github.com/spf13/cobra"
)

func newAddCommand(dbPath *string) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "add [text|-]",
		Short: "Store a memory",
		Long:  "Add a memory from an argument, --file path, or stdin pipe. Use '-' as text to read stdin explicitly.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			deps, cleanup, err := initRuntime(*dbPath, true, false)
			if err != nil {
				return err
			}
			defer cleanup()

			argText := ""
			if len(args) == 1 {
				argText = args[0]
			}
			text, err := readTextArgFileOrStdin(argText, filePath)
			if err != nil {
				return err
			}
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
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Read memory text from file path")

	return cmd
}
