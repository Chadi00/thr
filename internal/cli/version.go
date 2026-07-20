package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCommand(version string, commit string, buildDate string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "version.show", independentSelection(cmd), map[string]any{"version": version, "commit": commit, "build_date": buildDate}, nil)
			}
			fmt.Fprintln(cmd.OutOrStdout(), versionString(version, commit, buildDate))
			return nil
		},
	}
}
