package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Chadi00/thr/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	rootCmd := cli.NewRootCommand(version, commit, date)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	executed, err := rootCmd.ExecuteContextC(ctx)
	if err != nil {
		if !cli.PrintError(executed, err, os.Stderr) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
