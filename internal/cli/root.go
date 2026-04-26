package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewRootCommand(version string, commit string, buildDate string) *cobra.Command {
	var dbPath string

	rootCmd := &cobra.Command{
		Use:          "thr",
		Short:        "Save and find local memories",
		Long:         "Store local memories and retrieve them with keyword or semantic search.",
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
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Path to SQLite database (overrides THR_DB; default ~/.thr/thr.db)")
	rootCmd.PersistentFlags().Bool("json", false, "Emit JSON output for read-oriented commands")
	rootCmd.Flags().BoolP("version", "v", false, "Print version information")

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
		newPrefetchCommand(&dbPath),
		newIndexCommand(&dbPath),
		newSetupCommand(),
	)

	return rootCmd
}

func versionString(version string, commit string, buildDate string) string {
	return fmt.Sprintf("%s (commit=%s, date=%s)", version, commit, buildDate)
}
