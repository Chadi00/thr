package cli

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/repoctx"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newScopeCommand(dbPath *string) *cobra.Command {
	cmd := &cobra.Command{Use: "scope", Short: "Inspect and manage memory scopes", Args: cobra.NoArgs}
	cmd.AddCommand(
		newScopeListCommand(dbPath), newScopeShowCommand(dbPath), newScopeCreateCommand(dbPath),
		newScopeBindCommand(dbPath), newScopeUnbindCommand(dbPath), newScopeRebindCommand(dbPath),
		newScopeRenameCommand(dbPath), newScopeSplitCommand(dbPath),
	)
	return cmd
}

func newScopeListCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List registered scopes", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, selection, cleanup, err := resolveReadRuntime(cmd, *dbPath, nil, true)
			if err != nil {
				return err
			}
			defer cleanup()
			infos := []store.ScopeInfo{}
			if deps.repo != nil {
				infos, err = deps.repo.ListScopes(cmd.Context())
				if err != nil {
					return err
				}
			}
			currentID, drift := currentScopeID(cmd, deps.repo)
			if isJSONV2Output(cmd) {
				rows := make([]map[string]any, len(infos))
				for i, info := range infos {
					warnings := []output.Warning{}
					if info.ID == currentID && drift {
						warnings = append(warnings, output.Warning{Code: "repository_identity_drift", Message: "The current checkout remote differs from stored identity."})
					}
					rows[i] = map[string]any{"id": info.ID, "kind": info.Kind, "label": info.Label, "memory_count": info.MemoryCount, "latest_updated_at": info.Latest, "aliases": info.Aliases, "bindings": bindingDTOs(info.Bindings), "current": info.ID == currentID, "warnings": warnings}
				}
				return encodeV2(cmd, "scope.list", selection, map[string]any{"scopes": rows}, nil)
			}
			if len(infos) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No scopes are registered.")
			} else {
				table := output.NewTable(cmd.OutOrStdout())
				fmt.Fprintln(table, "SCOPE\tTYPE\tMEMORIES\tLABEL\tCURRENT")
				for _, info := range infos {
					label := output.SanitizeInline(info.Label)
					if info.Label == "" || info.Label == info.ID {
						label = "-"
					}
					fmt.Fprintf(table, "%s\t%s\t%d\t%s\t%t\n", output.SanitizeInline(info.ID), scopeType(info.Kind), info.MemoryCount, label, info.ID == currentID)
				}
				_ = table.Flush()
			}
			warnings := selectionWarnings(selection, nil)
			if drift {
				warnings = append(warnings, output.Warning{Code: "repository_identity_drift", Message: "The current checkout remote differs from its stored identity."})
			}
			printWarnings(cmd, warnings)
			return nil
		},
	}
}

func newScopeShowCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use: "show <scope>", Short: "Show one scope", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, selection, cleanup, err := resolveReadRuntime(cmd, *dbPath, []string{args[0]}, false)
			if err != nil {
				return err
			}
			defer cleanup()
			if deps.repo == nil {
				scope := selection.Scopes[0]
				if isJSONV2Output(cmd) {
					return encodeV2(cmd, "scope.show", selection, map[string]any{"scope": map[string]any{"id": scope.ID, "kind": scope.Kind, "label": scope.Label, "memory_count": 0, "latest_updated_at": nil, "aliases": []string{}, "bindings": []map[string]any{}, "current": false, "warnings": []output.Warning{}}}, nil)
				}
				printScopeDetails(cmd.OutOrStdout(), store.ScopeInfo{Scope: scope}, false)
				printHumanWarnings(cmd, selection, nil)
				return nil
			}
			infos, err := deps.repo.ListScopes(cmd.Context())
			if err != nil {
				return err
			}
			for _, info := range infos {
				if info.ID != selection.Scopes[0].ID {
					continue
				}
				if isJSONV2Output(cmd) {
					currentID, drift := currentScopeID(cmd, deps.repo)
					warnings := []output.Warning{}
					if info.ID == currentID && drift {
						warnings = append(warnings, output.Warning{Code: "repository_identity_drift", Message: "The current checkout remote differs from stored identity."})
					}
					return encodeV2(cmd, "scope.show", selection, map[string]any{"scope": map[string]any{"id": info.ID, "kind": info.Kind, "label": info.Label, "memory_count": info.MemoryCount, "latest_updated_at": info.Latest, "aliases": info.Aliases, "bindings": bindingDTOs(info.Bindings), "current": info.ID == currentID, "warnings": warnings}}, nil)
				}
				currentID, drift := currentScopeID(cmd, deps.repo)
				printScopeDetails(cmd.OutOrStdout(), info, info.ID == currentID)
				warnings := selectionWarnings(selection, nil)
				if info.ID == currentID && drift {
					warnings = append(warnings, output.Warning{Code: "repository_identity_drift", Message: "The current checkout remote differs from its stored identity."})
				}
				printWarnings(cmd, warnings)
				return nil
			}
			return scopeNotFound(args[0])
		},
	}
}

func newScopeCreateCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use: "create repo", Short: "Create and bind a repository scope", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "repo" {
				return &commandError{Code: "scope_selector_invalid", Message: "only repository scopes can be created"}
			}
			deps, selection, cleanup, err := resolveWriteRuntime(cmd, *dbPath, "repo")
			if err != nil {
				return err
			}
			defer cleanup()
			if selection.Resolution.Scope != nil {
				return &commandError{Code: "scope_conflict", Message: "repository already matches a scope; use scope bind or scope split"}
			}
			scope, err := deps.repo.EnsureRepositoryScope(cmd.Context(), *selection.Observation)
			if err != nil {
				return err
			}
			selection.Scopes = []domain.Scope{scope}
			return printScopeMutation(cmd, "scope.create", selection, scope, "Created")
		},
	}
}

func newScopeBindCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use: "bind <repo:id>", Short: "Bind the current checkout to a scope", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, selection, cleanup, err := resolveWriteRuntime(cmd, *dbPath, args[0])
			if err != nil {
				return err
			}
			defer cleanup()
			observation, err := observeRequired(cmd)
			if err != nil {
				return err
			}
			binding, err := deps.repo.BindRepository(cmd.Context(), selection.Scopes[0].ID, observation)
			if err != nil {
				return err
			}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "scope.bind", selection, map[string]any{"binding": bindingDTO(binding)}, nil)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Bound checkout %s to scope %s.\n", output.SanitizeInline(binding.CommonDir), output.SanitizeInline(binding.ScopeID))
			printHumanWarnings(cmd, selection, nil)
			return nil
		},
	}
}

func newScopeUnbindCommand(dbPath *string) *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use: "unbind", Short: "Unbind the current checkout", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, selection, cleanup, err := resolveExactRuntime(cmd, *dbPath, true)
			if err != nil {
				return err
			}
			defer cleanup()
			observation, err := observeRequired(cmd)
			if err != nil {
				return err
			}
			binding, err := deps.repo.UnbindRepository(cmd.Context(), observation, confirm)
			if errors.Is(err, store.ErrConfirmationRequired) {
				return &commandError{Code: "scope_conflict", Message: "unbinding would orphan a local-only scope; rerun with --confirm-orphan", SuggestedCommand: "thr scope unbind --confirm-orphan"}
			}
			if err != nil {
				return err
			}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "scope.unbind", selection, map[string]any{"binding": bindingDTO(binding)}, nil)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unbound checkout %s from scope %s.\n", output.SanitizeInline(binding.CommonDir), output.SanitizeInline(binding.ScopeID))
			printHumanWarnings(cmd, selection, nil)
			return nil
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm-orphan", false, "Confirm unbinding the last local-only checkout")
	return cmd
}

func newScopeRebindCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use: "rebind <repo:id>", Short: "Transfer the current checkout binding", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, selection, cleanup, err := resolveWriteRuntime(cmd, *dbPath, args[0])
			if err != nil {
				return err
			}
			defer cleanup()
			observation, err := observeRequired(cmd)
			if err != nil {
				return err
			}
			old, updated, err := deps.repo.RebindRepository(cmd.Context(), selection.Scopes[0].ID, observation)
			if err != nil {
				return err
			}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "scope.rebind", selection, map[string]any{"old": bindingDTO(old), "new": bindingDTO(updated)}, nil)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Rebound checkout %s from scope %s to %s.\n", output.SanitizeInline(updated.CommonDir), output.SanitizeInline(old.ScopeID), output.SanitizeInline(updated.ScopeID))
			printHumanWarnings(cmd, selection, nil)
			return nil
		},
	}
}

func newScopeRenameCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use: "rename <repo:id> <label>", Short: "Rename a repository scope", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, selection, cleanup, err := resolveExactRuntime(cmd, *dbPath, true)
			if err != nil {
				return err
			}
			defer cleanup()
			scope, err := deps.repo.RenameScope(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			selection.Scopes = []domain.Scope{scope}
			return printScopeMutation(cmd, "scope.rename", selection, scope, "Renamed")
		},
	}
}

func newScopeSplitCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use: "split", Short: "Split the current checkout into a new empty scope", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, selection, cleanup, err := resolveExactRuntime(cmd, *dbPath, true)
			if err != nil {
				return err
			}
			defer cleanup()
			observation, err := observeRequired(cmd)
			if err != nil {
				return err
			}
			scope, binding, err := deps.repo.SplitRepository(cmd.Context(), observation)
			if err != nil {
				return err
			}
			selection.Scopes = []domain.Scope{scope}
			if isJSONV2Output(cmd) {
				return encodeV2(cmd, "scope.split", selection, map[string]any{"scope": output.NewScopeDTO(scope), "binding": bindingDTO(binding)}, nil)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Split checkout %s into new scope %s.\n", output.SanitizeInline(binding.CommonDir), output.SanitizeInline(scope.ID))
			printHumanWarnings(cmd, selection, nil)
			return nil
		},
	}
}

func observeRequired(cmd *cobra.Command) (repoctx.Observation, error) {
	cwd, _ := cmd.Flags().GetString("cwd")
	observation, err := repoctx.Observe(cmd.Context(), cwd)
	if err != nil {
		return repoctx.Observation{}, &commandError{Code: "repository_unavailable", Message: "Git could not safely resolve repository context", Cause: err}
	}
	return observation, nil
}

func printScopeMutation(cmd *cobra.Command, operation string, selection resolvedSelection, scope domain.Scope, verb string) error {
	if isJSONV2Output(cmd) {
		return encodeV2(cmd, operation, selection, map[string]any{"scope": output.NewScopeDTO(scope)}, nil)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s scope %s (label: %s).\n", verb, output.SanitizeInline(scope.ID), output.SanitizeInline(scope.Label))
	printHumanWarnings(cmd, selection, nil)
	return nil
}

func printScopeDetails(w io.Writer, info store.ScopeInfo, current bool) {
	latest := "never"
	if info.Latest != nil {
		latest = info.Latest.Format(time.RFC3339)
	}
	fmt.Fprintf(w, "ID: %s\n", output.SanitizeInline(info.ID))
	fmt.Fprintf(w, "Type: %s\n", scopeType(info.Kind))
	fmt.Fprintf(w, "Label: %s\n", output.SanitizeInline(info.Label))
	fmt.Fprintf(w, "Memories: %d\n", info.MemoryCount)
	fmt.Fprintf(w, "Last updated: %s\n", latest)
	fmt.Fprintf(w, "Current repository: %t\n", current)
	if len(info.Aliases) > 0 {
		fmt.Fprintln(w, "Aliases:")
		for _, alias := range info.Aliases {
			fmt.Fprintf(w, "  - %s\n", output.SanitizeInline(alias))
		}
	}
	if len(info.Bindings) > 0 {
		fmt.Fprintln(w, "Bindings:")
		table := output.NewTable(w)
		fmt.Fprintln(table, "GIT DIRECTORY\tWORKTREE\tREMOTE\tACTIVE")
		for _, binding := range info.Bindings {
			remote := output.SanitizeInline(binding.CanonicalRemote)
			if remote == "" {
				remote = "-"
			}
			fmt.Fprintf(table, "%s\t%s\t%s\t%t\n", output.SanitizeInline(binding.CommonDir), output.SanitizeInline(binding.WorktreeRoot), remote, binding.Active)
		}
		_ = table.Flush()
	}
}

func scopeType(kind domain.ScopeKind) string {
	if kind == domain.ScopeKindRepo {
		return "repository"
	}
	return "user"
}

func bindingDTO(binding store.RepositoryBinding) map[string]any {
	return map[string]any{
		"id": binding.ID, "scope_id": binding.ScopeID, "common_dir": binding.CommonDir,
		"worktree_root": binding.WorktreeRoot, "remote_name": binding.RemoteName,
		"remote_url": binding.RemoteURL, "canonical_remote": binding.CanonicalRemote, "active": binding.Active,
	}
}

func bindingDTOs(bindings []store.RepositoryBinding) []map[string]any {
	result := make([]map[string]any, len(bindings))
	for i, binding := range bindings {
		result[i] = bindingDTO(binding)
	}
	return result
}

func currentScopeID(cmd *cobra.Command, repo *store.Repository) (string, bool) {
	if repo == nil {
		return "", false
	}
	observation, err := observeRequired(cmd)
	if err != nil {
		return "", false
	}
	resolution, err := repo.ResolveRepository(cmd.Context(), observation)
	if err != nil || resolution.Scope == nil {
		return "", false
	}
	return resolution.Scope.ID, resolution.Drift
}
