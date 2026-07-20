package cli

import (
	"fmt"
	"strconv"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newEditCommand(dbPath *string) *cobra.Command {
	var maxBytes int64
	var ifScope string
	var ifRevision int64

	cmd := &cobra.Command{
		Use:   "edit <id> <text|->",
		Short: "Replace a memory",
		Long:  "Replace a memory using text. Use '-' to read replacement text from stdin explicitly.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}

			text, err := readTextArgOrExplicitStdin(args[1], maxBytes)
			if err != nil {
				return err
			}

			deps, selection, cleanup, err := resolveExactRuntime(cmd, *dbPath, true)
			if err != nil {
				if isMissingDatabase(err) {
					return &commandError{Code: "memory_not_found", Message: fmt.Sprintf("memory %d not found", id), Cause: store.ErrMemoryNotFound}
				}
				return err
			}
			defer cleanup()
			if _, err := deps.repo.GetMemory(cmd.Context(), id); err != nil {
				return &commandError{Code: "memory_not_found", Message: fmt.Sprintf("memory %d not found", id), Cause: err}
			}
			if err := initEmbedder(deps, false); err != nil {
				return err
			}

			embedding, err := deps.embedder.PassageEmbed(text)
			if err != nil {
				return fmt.Errorf("embed memory text: %w", err)
			}

			memory, err := deps.repo.EditMemory(cmd.Context(), id, text, embedding, activeEmbeddingIdentity(), store.Preconditions{ScopeID: ifScope, Revision: ifRevision})
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
				return encodeV2(cmd, "memory.edit", selection, map[string]any{"memory": output.NewMemoryDTO(memory)}, []domain.Memory{memory})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated memory %d in %s\n", memory.ID, memory.Scope.ID)
			return nil
		},
	}

	cmd.Flags().Int64Var(&maxBytes, "max-bytes", defaultMaxMemoryBytes, "Maximum memory text size in bytes")
	cmd.Flags().StringVar(&ifScope, "if-scope", "", "Require the memory to remain in this scope")
	cmd.Flags().Int64Var(&ifRevision, "if-revision", 0, "Require the current memory revision")

	return cmd
}
