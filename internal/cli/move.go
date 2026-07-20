package cli

import (
	"fmt"
	"strconv"

	"github.com/Chadi00/thr/internal/config"
	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newMoveCommand(dbPath *string) *cobra.Command {
	var destinations []string
	var ifScope string
	var ifRevision int64
	cmd := &cobra.Command{
		Use:   "move <id>",
		Short: "Move a memory to another scope without re-embedding it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(destinations) != 1 {
				return &commandError{Code: "scope_selector_invalid", Message: "move requires exactly one --to scope"}
			}
			destination := destinations[0]
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}
			if destination == "" || !validSelector(destination) {
				return &commandError{Code: "scope_selector_invalid", Message: "move requires one valid --to scope"}
			}
			cfg, err := config.Load(*dbPath)
			if err != nil {
				return err
			}
			info, err := store.InspectOperationalDatabase(cfg.DBPath)
			if err != nil {
				return err
			}
			if info.Status == store.DatabaseMissing {
				return &commandError{Code: "memory_not_found", Message: fmt.Sprintf("memory %d not found", id), Cause: store.ErrMemoryNotFound}
			}
			deps, selection, cleanup, err := resolveWriteRuntime(cmd, *dbPath, destination)
			if err != nil {
				return err
			}
			defer cleanup()
			pre := store.Preconditions{ScopeID: ifScope, Revision: ifRevision}
			var result store.MoveResult
			if destination == "repo" {
				result, err = deps.repo.MoveMemoryToRepository(cmd.Context(), id, *selection.Observation, pre)
			} else {
				result, err = deps.repo.MoveMemory(cmd.Context(), id, selection.Scopes[0].ID, pre)
			}
			if err != nil {
				switch err {
				case store.ErrMemoryNotFound:
					return &commandError{Code: "memory_not_found", Message: fmt.Sprintf("memory %d not found", id), Cause: err}
				case store.ErrScopeConflict, store.ErrRevisionConflict:
					return &commandError{Code: "scope_conflict", Message: "memory precondition failed", Cause: err}
				}
				return err
			}
			selection.Scopes = []domain.Scope{result.To}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "memory.move", selection, map[string]any{
					"memory": output.NewMemoryDTO(result.Memory), "from": output.NewScopeDTO(result.From), "to": output.NewScopeDTO(result.To),
				}, []domain.Memory{result.Memory})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "moved memory %d from [%s] to [%s]\n", id, result.From.ID, result.To.ID)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&destinations, "to", nil, "Destination scope: user, repo, or repo:<id>")
	cmd.Flags().StringVar(&ifScope, "if-scope", "", "Require the memory to remain in this scope")
	cmd.Flags().Int64Var(&ifRevision, "if-revision", 0, "Require the current memory revision")
	return cmd
}
