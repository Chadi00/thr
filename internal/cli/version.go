package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCommand(version string, commit string, buildDate string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "thr version %s\n", versionString(version, commit, buildDate))
		},
	}
}
