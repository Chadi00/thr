package cli

import (
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	var dbPath string

	rootCmd := &cobra.Command{
		Use:   "thr",
		Short: "Tiny History Recall",
	}

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Path to SQLite database (default ~/.thr/thr.db)")

	rootCmd.AddCommand(
		newAddCommand(&dbPath),
		newListCommand(&dbPath),
		newAskCommand(&dbPath),
		newSearchCommand(&dbPath),
		newEditCommand(&dbPath),
		newForgetCommand(&dbPath),
	)

	return rootCmd
}
