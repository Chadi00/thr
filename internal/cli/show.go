package cli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newShowCommand(dbPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show one memory by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			deps, cleanup, err := initRuntime(*dbPath, false, false)
			if err != nil {
				return err
			}
			defer cleanup()

			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}
			memory, err := deps.repo.GetMemory(ctx, id)
			if err != nil {
				if err == store.ErrMemoryNotFound {
					return fmt.Errorf("memory %d not found", id)
				}
				return err
			}

			if isJSONOutput(cmd) {
				return output.PrintMemoryJSON(cmd.OutOrStdout(), memory)
			}
			output.PrintMemory(cmd.OutOrStdout(), memory)
			return nil
		},
	}
	return cmd
}
