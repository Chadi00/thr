package cli

import (
	"fmt"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/embed"
	"github.com/Chadi00/thr/internal/output"
	"github.com/spf13/cobra"
)

func newAddCommand(dbPath *string) *cobra.Command {
	var maxBytes int64
	var scopeSelectors []string

	cmd := &cobra.Command{
		Use:   "add <text|->",
		Short: "Store a memory",
		Long:  "Add a memory from text. Use '-' to read from stdin explicitly.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(scopeSelectors) > 1 {
				return &commandError{Code: "scope_selector_invalid", Message: "add accepts exactly one --scope"}
			}
			scopeSelector := ""
			if len(scopeSelectors) == 1 {
				scopeSelector = scopeSelectors[0]
			}
			text, err := readTextArgOrExplicitStdin(args[0], maxBytes)
			if err != nil {
				return err
			}

			deps, selection, cleanup, err := resolveWriteRuntime(cmd, *dbPath, scopeSelector)
			if err != nil {
				return err
			}
			defer cleanup()
			if err := initEmbedder(deps, false); err != nil {
				return err
			}

			embedding, err := deps.embedder.PassageEmbed(text)
			if err != nil {
				return fmt.Errorf("embed memory text: %w", err)
			}

			var memory domain.Memory
			if scopeSelector == "" || scopeSelector == "repo" {
				assignment := domain.ScopeAssignmentAutomatic
				if scopeSelector == "repo" {
					assignment = domain.ScopeAssignmentExplicit
				}
				memory, err = deps.repo.AddRepositoryMemory(cmd.Context(), text, embedding, activeEmbeddingIdentity(), *selection.Observation, assignment)
			} else {
				memory, err = deps.repo.AddMemory(cmd.Context(), text, embedding, activeEmbeddingIdentity(), selection.Scopes[0].ID, domain.ScopeAssignmentExplicit)
			}
			if err != nil {
				return err
			}
			selection.Scopes = []domain.Scope{memory.Scope}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "memory.add", selection, map[string]any{"memory": output.NewMemoryDTO(memory)}, []domain.Memory{memory})
			}
			output.PrintMemoryAdded(cmd.OutOrStdout(), memory)
			printHumanWarnings(cmd, selection, []domain.Memory{memory})
			return nil
		},
	}

	cmd.Flags().Int64Var(&maxBytes, "max-bytes", defaultMaxMemoryBytes, fmt.Sprintf("Maximum input size in bytes (memory text limit: %d Unicode code points)", embed.MaxMemoryTextCodePoints))
	cmd.Flags().StringArrayVar(&scopeSelectors, "scope", nil, "Destination scope: user, repo, or repo:<id>")

	return cmd
}
