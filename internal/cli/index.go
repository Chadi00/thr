package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newIndexCommand(dbPath *string) *cobra.Command {
	var scopes []string
	var allScopes bool
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Update the semantic search index",
		Long:  "Rebuilds missing or stale semantic search embeddings for the active local model.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, selection, cleanup, err := resolveReadRuntime(cmd, *dbPath, scopes, allScopes || len(scopes) == 0)
			if err != nil {
				return err
			}
			defer cleanup()
			if deps.repo == nil {
				if isJSONV2Output(cmd) {
					return encodeV2(cmd, "memory.index", selection, map[string]any{"indexed": 0}, nil)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "no memories stored")
				return nil
			}
			// Reopen writable after read-only scope resolution.
			cleanup()
			deps, cleanup, err = initExistingWriteRuntime(*dbPath)
			if err != nil {
				return err
			}
			defer cleanup()
			if err := initEmbedder(deps, !isJSONV2Output(cmd)); err != nil {
				return err
			}

			identity := activeEmbeddingIdentity()
			memories, err := deps.repo.ListMemoriesNeedingIndex(cmd.Context(), selection.IDs(), identity)
			if err != nil {
				return err
			}
			if len(memories) == 0 {
				if isJSONV2Output(cmd) {
					return encodeV2(cmd, "memory.index", selection, map[string]any{"indexed": 0}, nil)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "semantic index is up to date")
				return nil
			}

			if !isJSONV2Output(cmd) {
				fmt.Fprintf(cmd.OutOrStdout(), "indexing %d memories\n", len(memories))
			}
			for i, memory := range memories {
				embedding, err := deps.embedder.PassageEmbed(memory.Text)
				if err != nil {
					return fmt.Errorf("embed memory %d: %w", memory.ID, err)
				}
				if err := deps.repo.UpsertMemoryEmbedding(cmd.Context(), memory.ID, embedding, identity, memory.Revision, memory.Scope.ID); err != nil {
					return err
				}
				if !isJSONV2Output(cmd) {
					fmt.Fprintf(cmd.OutOrStdout(), "indexed %d/%d\n", i+1, len(memories))
				}
			}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "memory.index", selection, map[string]any{"indexed": len(memories)}, memories)
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&scopes, "scope", nil, "Exact scope to index; repeat for a union")
	cmd.Flags().BoolVar(&allScopes, "all-scopes", false, "Index every registered scope")
	return cmd
}
