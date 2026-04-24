package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewRootCommand(version string, commit string, buildDate string) *cobra.Command {
	var dbPath string

	rootCmd := &cobra.Command{
		Use:          "thr",
		Short:        "Tiny History Recall",
		Long:         "Tiny History Recall stores local memories and retrieves them with keyword or semantic search (retrieval only, no LLM-generated answers).",
		SilenceUsage: true,
		Version:      versionString(version, commit, buildDate),
		RunE: func(cmd *cobra.Command, args []string) error {
			showVersion, err := cmd.Flags().GetBool("version")
			if err != nil {
				return err
			}
			if showVersion {
				fmt.Fprintln(cmd.OutOrStdout(), versionString(version, commit, buildDate))
				return nil
			}
			return cmd.Help()
		},
	}

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Path to SQLite database (overrides THR_DB; default ~/.thr/thr.db)")
	rootCmd.PersistentFlags().Bool("json", false, "Emit JSON output for read-oriented commands")
	rootCmd.Flags().BoolP("version", "v", false, "Print version information")
	_ = rootCmd.RegisterFlagCompletionFunc("db", cobra.NoFileCompletions)

	rootCmd.AddCommand(
		newAddCommand(&dbPath),
		newListCommand(&dbPath),
		newAskCommand(&dbPath),
		newSearchCommand(&dbPath),
		newEditCommand(&dbPath),
		newShowCommand(&dbPath),
		newForgetCommand(&dbPath),
		newStatsCommand(&dbPath),
		newVersionCommand(version, commit, buildDate),
		newCompletionCommand(),
		newPrefetchCommand(&dbPath),
	)

	return rootCmd
}

func versionString(version string, commit string, buildDate string) string {
	return fmt.Sprintf("%s (commit=%s, date=%s)", version, commit, buildDate)
}
