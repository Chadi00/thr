package cli

import (
	"fmt"
	"strconv"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newForgetCommand(dbPath *string) *cobra.Command {
	var ifScope string
	var ifRevision int64
	cmd := &cobra.Command{
		Use:   "forget <id>",
		Short: "Delete a memory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}

			deps, selection, cleanup, err := resolveExactRuntime(cmd, *dbPath, true)
			if err != nil {
				if isMissingDatabase(err) {
					return &commandError{Code: "memory_not_found", Message: fmt.Sprintf("memory %d not found", id), Cause: store.ErrMemoryNotFound}
				}
				return err
			}
			defer cleanup()

			memory, err := deps.repo.ForgetMemory(cmd.Context(), id, store.Preconditions{ScopeID: ifScope, Revision: ifRevision})
			if err != nil {
				if err == store.ErrMemoryNotFound {
					return &commandError{Code: "memory_not_found", Message: fmt.Sprintf("memory %d not found", id), Cause: err}
				}
				if err == store.ErrScopeConflict || err == store.ErrRevisionConflict {
					return &commandError{Code: "scope_conflict", Message: "memory precondition failed", Cause: err}
				}
				return err
			}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "memory.forget", selection, map[string]any{"memory": output.NewMemoryDTO(memory)}, []domain.Memory{memory})
			}
			output.PrintForget(cmd.OutOrStdout(), memory)
			return nil
		},
	}
	cmd.Flags().StringVar(&ifScope, "if-scope", "", "Require the memory to remain in this scope")
	cmd.Flags().Int64Var(&ifRevision, "if-revision", 0, "Require the current memory revision")

	return cmd
}
