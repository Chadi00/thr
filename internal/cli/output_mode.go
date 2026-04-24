package cli

import "github.com/spf13/cobra"

func isJSONOutput(cmd *cobra.Command) bool {
	value, err := cmd.Flags().GetBool("json")
	if err == nil {
		return value
	}
	value, err = cmd.InheritedFlags().GetBool("json")
	return err == nil && value
}
