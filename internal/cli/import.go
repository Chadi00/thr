package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
)

type importMemory struct {
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newImportCommand(dbPath *string) *cobra.Command {
	var filePath string

	cmd := &cobra.Command{
		Use:   "import [file|-]",
		Short: "Import memories from JSONL",
		Long:  "Import memories from JSONL created by `thr export`. Incoming ids are ignored and new ids are assigned.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			deps, cleanup, err := initRuntime(*dbPath, true, false)
			if err != nil {
				return err
			}
			defer cleanup()

			reader, closeFn, err := importReader(args, filePath)
			if err != nil {
				return err
			}
			defer closeFn()

			scanner := bufio.NewScanner(reader)
			count := 0
			for scanner.Scan() {
				line := scanner.Bytes()
				if len(line) == 0 {
					continue
				}
				var record importMemory
				if err := json.Unmarshal(line, &record); err != nil {
					return fmt.Errorf("decode import line %d: %w", count+1, err)
				}
				if record.Text == "" {
					return fmt.Errorf("decode import line %d: missing text", count+1)
				}
				if record.CreatedAt.IsZero() {
					record.CreatedAt = time.Now().UTC()
				}
				if record.UpdatedAt.IsZero() {
					record.UpdatedAt = record.CreatedAt
				}

				embedding, err := deps.embedder.PassageEmbed(record.Text)
				if err != nil {
					return fmt.Errorf("embed import memory line %d: %w", count+1, err)
				}
				if _, err := deps.repo.ImportMemory(ctx, record.Text, record.CreatedAt, record.UpdatedAt, embedding); err != nil {
					return fmt.Errorf("store import memory line %d: %w", count+1, err)
				}
				count++
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read import stream: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "imported %d memories\n", count)
			return nil
		},
	}
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "Read JSONL import data from file path")
	return cmd
}

func importReader(args []string, filePath string) (io.Reader, func(), error) {
	if filePath != "" && len(args) == 1 {
		return nil, func() {}, fmt.Errorf("use either positional file or --file, not both")
	}
	if filePath != "" {
		f, err := os.Open(filePath)
		if err != nil {
			return nil, func() {}, fmt.Errorf("open import file %q: %w", filePath, err)
		}
		return f, func() { _ = f.Close() }, nil
	}
	if len(args) == 1 {
		if args[0] == "-" {
			return os.Stdin, func() {}, nil
		}
		f, err := os.Open(args[0])
		if err != nil {
			return nil, func() {}, fmt.Errorf("open import file %q: %w", args[0], err)
		}
		return f, func() { _ = f.Close() }, nil
	}
	if hasStdinInput() {
		return os.Stdin, func() {}, nil
	}
	return nil, func() {}, fmt.Errorf("no import source provided; pass file path, --file, '-', or pipe stdin")
}
