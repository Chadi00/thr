package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Chadi00/thr/internal/config"
	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func independentSelection(cmd *cobra.Command) resolvedSelection {
	dbFlag, _ := cmd.Flags().GetString("db")
	cfg, _ := config.Load(dbFlag)
	database, _ := store.InspectDatabase(cfg.DBPath)
	cwd, _ := cmd.Flags().GetString("cwd")
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	cwd, _ = filepath.Abs(cwd)
	return resolvedSelection{CWD: cwd, Database: database}
}

func encodeV2(cmd *cobra.Command, command string, selection resolvedSelection, result any, memories []domain.Memory) error {
	warnings := selectionWarnings(selection, memories)
	return output.EncodeJSONV2(cmd.OutOrStdout(), output.Envelope{
		OK: true, Command: command, Context: outputContext(selection), Result: result, Warnings: warnings,
	})
}

func outputContext(selection resolvedSelection) output.Context {
	context := output.Context{
		Database: output.Database{Path: selection.Database.Path, Status: string(selection.Database.Status)},
		CWD:      selection.CWD,
	}
	if selection.Mode != "" {
		context.ScopeSelection = &output.ScopeSelection{
			Mode: selection.Mode, Requested: selection.Requested, Resolved: selection.IDs(),
		}
	}
	return context
}

func selectionWarnings(selection resolvedSelection, memories []domain.Memory) []output.Warning {
	warnings := append([]output.Warning{}, selection.Warnings...)
	if selection.Migration != nil && selection.Migration.BackupPath != "" {
		warnings = append(warnings, output.Warning{
			Code: "database_migrated", Message: "The legacy database was backed up and migrated to memory scopes.",
			Details: map[string]any{"backup_path": selection.Migration.BackupPath},
		})
	}
	legacyIDs := make([]string, 0)
	for _, memory := range memories {
		if memory.ScopeAssignment == domain.ScopeAssignmentLegacy {
			legacyIDs = append(legacyIDs, fmt.Sprint(memory.ID))
		}
	}
	if len(legacyIDs) > 0 {
		warnings = append(warnings, output.Warning{
			Code: "legacy_scope_assignment", Message: "Returned memories include legacy user-scope assignments.",
			Details: map[string]any{"memory_ids": legacyIDs},
		})
	}
	if selection.Resolution.Drift {
		warnings = append(warnings, output.Warning{Code: "repository_identity_drift", Message: "The bound repository remote differs from its stored identity."})
	}
	return warnings
}

func printHumanWarnings(cmd *cobra.Command, selection resolvedSelection, memories []domain.Memory) {
	for _, memory := range memories {
		if memory.ScopeAssignment == domain.ScopeAssignmentLegacy {
			fmt.Fprintln(cmd.OutOrStdout(), "warning: legacy memories are assigned to [user]; review with 'thr list --scope user --legacy'")
			break
		}
	}
}

func printAutomaticMigration(cmd *cobra.Command, selection resolvedSelection) {
	mode, _ := commandOutputMode(cmd)
	if mode == outputHuman && selection.Migration != nil && selection.Migration.BackupPath != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "migrated legacy memories to [user]; backup: %s\n", output.SanitizeInline(selection.Migration.BackupPath))
	}
}

func semanticDTOs(results []store.SemanticHit) []output.SemanticMatchDTO {
	dtos := make([]output.SemanticMatchDTO, len(results))
	for i, result := range results {
		dtos[i] = output.NewSemanticMatchDTO(i+1, result)
	}
	return dtos
}

func memoryDTOs(memories []domain.Memory) []output.MemoryDTO {
	dtos := make([]output.MemoryDTO, len(memories))
	for i, memory := range memories {
		dtos[i] = output.NewMemoryDTO(memory)
	}
	return dtos
}
