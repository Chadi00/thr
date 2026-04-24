package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newVacuumCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "vacuum",
		Short: "Run VACUUM on the active database",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			deps, cleanup, err := initRuntime(*dbPath, false, false)
			if err != nil {
				return err
			}
			defer cleanup()
			if err := deps.repo.Vacuum(ctx); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "vacuum completed")
			return nil
		},
	}
}
