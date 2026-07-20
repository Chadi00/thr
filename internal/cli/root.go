package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewRootCommand(version string, commit string, buildDate string) *cobra.Command {
	var dbPath string

	rootCmd := &cobra.Command{
		Use:           "thr",
		Short:         "Save and find local memories",
		Long:          "Store local memories and retrieve them with keyword or semantic search.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       versionString(version, commit, buildDate),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			_, err := commandOutputMode(cmd)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			showVersion, err := cmd.Flags().GetBool("version")
			if err != nil {
				return err
			}
			if showVersion {
				if isJSONV2Output(cmd) {
					return encodeV2(cmd, "version.show", independentSelection(cmd), map[string]any{"version": version, "commit": commit, "build_date": buildDate}, nil)
				}
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
	rootCmd.PersistentFlags().String("format", "", "Output format: human, legacy-json, or json-v2")
	rootCmd.PersistentFlags().String("cwd", "", "Repository context directory (default current directory)")
	rootCmd.Flags().BoolP("version", "v", false, "Print version information")

	rootCmd.AddCommand(
		newAddCommand(&dbPath),
		newListCommand(&dbPath),
		newAskCommand(&dbPath),
		newSearchCommand(&dbPath),
		newEditCommand(&dbPath),
		newShowCommand(&dbPath),
		newForgetCommand(&dbPath),
		newMoveCommand(&dbPath),
		newStatsCommand(&dbPath),
		newContextCommand(&dbPath),
		newScopeCommand(&dbPath),
		newMigrateCommand(&dbPath),
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
