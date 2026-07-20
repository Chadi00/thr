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
				fmt.Fprintf(cmd.OutOrStdout(), "No database exists at %s; nothing to migrate.\n", output.SanitizeInline(cfg.DBPath))
				printManagedSkillWarnings(cmd, selection.Warnings)
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
				fmt.Fprintln(cmd.OutOrStdout(), "Database is already current; nothing to migrate.")
				printManagedSkillWarnings(cmd, selection.Warnings)
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Database migration complete.")
			fmt.Fprintf(cmd.OutOrStdout(), "Memories migrated: %d\n", result.Memories)
			fmt.Fprintln(cmd.OutOrStdout(), "Destination scope: [user]")
			fmt.Fprintf(cmd.OutOrStdout(), "Backup: %s\n", output.SanitizeInline(result.BackupPath))
			printManagedSkillWarnings(cmd, selection.Warnings)
			return nil
		},
	}
}

func printManagedSkillWarnings(cmd *cobra.Command, warnings []output.Warning) {
	printWarnings(cmd, warnings)
}
