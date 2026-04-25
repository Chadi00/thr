package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newIndexCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "index",
		Short: "Update the semantic search index",
		Long:  "Rebuilds missing or stale semantic search embeddings for the active local model.",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, cleanup, err := initExistingWriteRuntimeWithEmbedder(*dbPath, true)
			if err != nil {
				if isMissingDatabase(err) {
					fmt.Fprintln(cmd.OutOrStdout(), "no memories stored")
					return nil
				}
				return err
			}
			defer cleanup()

			identity := activeEmbeddingIdentity()
			memories, err := deps.repo.ListMemoriesNeedingIndex(cmd.Context(), identity)
			if err != nil {
				return err
			}
			if len(memories) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "semantic index is up to date")
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "indexing %d memories\n", len(memories))
			for i, memory := range memories {
				embedding, err := deps.embedder.PassageEmbed(memory.Text)
				if err != nil {
					return fmt.Errorf("embed memory %d: %w", memory.ID, err)
				}
				if err := deps.repo.UpsertMemoryEmbedding(cmd.Context(), memory.ID, embedding, identity); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "indexed %d/%d\n", i+1, len(memories))
			}
			return nil
		},
	}
}
