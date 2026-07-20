package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Chadi00/thr/internal/config"
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newMigrateCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Back up and migrate a legacy database to memory scopes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*dbPath)
			if err != nil {
				return err
			}
			cwd, _ := cmd.Flags().GetString("cwd")
			if cwd == "" {
				cwd, _ = os.Getwd()
			}
			cwd, _ = filepath.Abs(cwd)
			info, err := store.InspectOperationalDatabase(cfg.DBPath)
			if err != nil {
				return err
			}
			selection := resolvedSelection{CWD: cwd, Database: info, Warnings: managedSkillWarnings()}
			if info.Status == store.DatabaseMissing {
				if isJSONV2Output(cmd) {
					return encodeV2(cmd, "database.migrate", selection, map[string]any{"migrated": false}, nil)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "no database to migrate")
				return nil
			}
			result, err := store.MigratePath(cmd.Context(), cfg.DBPath)
			if err != nil {
				if errors.Is(err, store.ErrDatabaseIncompatible) {
					return &commandError{Code: "database_version_incompatible", Message: "the selected database format is incompatible", Cause: err}
				}
				return &commandError{Code: "database_migration_required", Message: "database migration failed", SuggestedCommand: "thr migrate", Cause: err}
			}
			selection.Database.Status = store.DatabaseCompatible
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "database.migrate", selection, map[string]any{
					"migrated": result.OldFormat != result.NewFormat, "backup_path": result.BackupPath,
					"old_format": result.OldFormat, "new_format": result.NewFormat,
					"memories": result.Memories, "embeddings": result.Embeddings,
				}, nil)
			}
			if result.BackupPath == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "database is already current")
				printManagedSkillWarnings(cmd, selection.Warnings)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "migrated %d memories to [user]\nbackup\t%s\n", result.Memories, output.SanitizeInline(result.BackupPath))
			printManagedSkillWarnings(cmd, selection.Warnings)
			return nil
		},
	}
}

func printManagedSkillWarnings(cmd *cobra.Command, warnings []output.Warning) {
	for _, warning := range warnings {
		fmt.Fprintf(cmd.OutOrStdout(), "warning: %s; run %s\n", warning.Message, warning.Details["suggested_command"])
	}
}
