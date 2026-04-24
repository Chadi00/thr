package main

import (
	"fmt"
	"os"

	"github.com/Chadi00/thr/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	rootCmd := cli.NewRootCommand(version, commit, date)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
