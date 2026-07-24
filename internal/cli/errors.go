package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/Chadi00/thr/internal/config"
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func PrintError(cmd *cobra.Command, err error, writer io.Writer) bool {
	mode, modeErr := commandOutputMode(cmd)
	if modeErr != nil || mode != outputJSONV2 {
		message := err.Error()
		var commandErr *commandError
		if errors.As(err, &commandErr) {
			message = commandErr.Message
		}
		_, _ = io.WriteString(writer, "Error: "+output.SanitizeInline(message)+"\n")
		if commandErr != nil && commandErr.SuggestedCommand != "" {
			_, _ = io.WriteString(writer, "Next: "+output.SanitizeInline(commandErr.SuggestedCommand)+"\n")
		}
		return true
	}
	dbFlag, _ := cmd.Flags().GetString("db")
	cfg, _ := config.Load(dbFlag)
	if cfg.DBPath == "" {
		cfg.DBPath = dbFlag
	}
	database, _ := store.InspectDatabase(cfg.DBPath)
	cwd, _ := cmd.Flags().GetString("cwd")
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	cwd, _ = filepath.Abs(cwd)
	structured := &output.StructuredError{Code: "internal_error", Message: err.Error(), Retryable: false}
	var commandErr *commandError
	if errors.As(err, &commandErr) {
		structured.Code = commandErr.Code
		structured.Message = commandErr.Message
		structured.SuggestedCommand = commandErr.SuggestedCommand
	}
	command := operationName(cmd)
	context := output.Context{Database: output.Database{Path: cfg.DBPath, Status: string(database.Status)}, CWD: cwd}
	if selection := errorScopeSelection(cmd); selection != nil {
		context.ScopeSelection = selection
	}
	warnings := []output.Warning{}
	if selection, ok := cmd.Context().Value(selectionContextKey{}).(resolvedSelection); ok {
		warnings = selectionWarnings(selection, nil)
	}
	_ = output.EncodeJSONV2(writer, output.Envelope{
		OK: false, Command: command,
		Context: context,
		Result:  nil, Error: structured, Warnings: warnings,
	})
	return true
}

func operationName(cmd *cobra.Command) string {
	switch cmd.CommandPath() {
	case "thr add":
		return "memory.add"
	case "thr ask":
		return "memory.ask"
	case "thr search":
		return "memory.search"
	case "thr list":
		return "memory.list"
	case "thr show":
		return "memory.show"
	case "thr edit":
		return "memory.edit"
	case "thr forget":
		return "memory.forget"
	case "thr move":
		return "memory.move"
	case "thr stats":
		return "memory.stats"
	case "thr index":
		return "memory.index"
	case "thr context":
		return "context.show"
	case "thr migrate":
		return "database.migrate"
	case "thr update":
		return "software.update"
	}
	if cmd.Parent() != nil && cmd.Parent().Name() == "scope" {
		return "scope." + cmd.Name()
	}
	if cmd.Parent() != nil && cmd.Parent().Name() == "setup" {
		return "setup." + cmd.Name()
	}
	if cmd.Name() == "prefetch" {
		return "model.prefetch"
	}
	if cmd.Name() == "version" {
		return "version.show"
	}
	return cmd.Name()
}

func errorScopeSelection(cmd *cobra.Command) *output.ScopeSelection {
	if selection, ok := cmd.Context().Value(selectionContextKey{}).(resolvedSelection); ok && selection.Mode != "" {
		return &output.ScopeSelection{Mode: selection.Mode, Requested: selection.Requested, Resolved: selection.IDs()}
	}
	mode := ""
	requested := []string{}
	if flag := cmd.Flags().Lookup("scope"); flag != nil {
		if values, err := cmd.Flags().GetStringArray("scope"); err == nil {
			requested = values
		} else if value, err := cmd.Flags().GetString("scope"); err == nil && value != "" {
			requested = []string{value}
		}
	}
	if flag := cmd.Flags().Lookup("all-scopes"); flag != nil {
		all, _ := cmd.Flags().GetBool("all-scopes")
		if all {
			mode = "all"
		}
	}
	if mode == "" && len(requested) > 0 {
		mode = "explicit"
	}
	if mode == "" {
		switch cmd.Name() {
		case "add":
			mode = "automatic_write"
		case "ask", "search", "list":
			mode = "automatic_read"
		case "stats", "index":
			mode = "all"
		}
	}
	if mode == "" {
		return nil
	}
	return &output.ScopeSelection{Mode: mode, Requested: requested, Resolved: []string{}}
}
