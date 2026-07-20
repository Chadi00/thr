package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type outputMode string

const (
	outputHuman      outputMode = "human"
	outputLegacyJSON outputMode = "legacy-json"
	outputJSONV2     outputMode = "json-v2"
)

func commandOutputMode(cmd *cobra.Command) (outputMode, error) {
	legacy, _ := cmd.Flags().GetBool("json")
	format, _ := cmd.Flags().GetString("format")
	if legacy && format != "" {
		return "", fmt.Errorf("--json and --format are mutually exclusive")
	}
	if legacy {
		return outputLegacyJSON, nil
	}
	switch outputMode(format) {
	case "", outputHuman:
		return outputHuman, nil
	case outputLegacyJSON, outputJSONV2:
		return outputMode(format), nil
	default:
		return "", fmt.Errorf("unsupported --format %q; use human, legacy-json, or json-v2", format)
	}
}

func isJSONOutput(cmd *cobra.Command) bool {
	mode, _ := commandOutputMode(cmd)
	return mode == outputLegacyJSON
}

func isJSONV2Output(cmd *cobra.Command) bool {
	mode, _ := commandOutputMode(cmd)
	return mode == outputJSONV2
}
