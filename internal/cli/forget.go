package cli

import (
	"fmt"
	"strconv"

	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newForgetCommand(dbPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forget <id>",
		Short: "Delete a memory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}

			deps, cleanup, err := initWriteRuntime(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()

			if err := deps.repo.ForgetMemory(cmd.Context(), id); err != nil {
				if err == store.ErrMemoryNotFound {
					return fmt.Errorf("memory %d not found", id)
				}
				return err
			}

			output.PrintForget(cmd.OutOrStdout(), id)
			return nil
		},
	}

	return cmd
}
