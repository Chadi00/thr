package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Chadi00/thr/internal/config"
	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/output"
	"github.com/Chadi00/thr/internal/repoctx"
	"github.com/Chadi00/thr/internal/store"
	"github.com/spf13/cobra"
)

func newContextCommand(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "context",
		Short: "Inspect repository and memory scope context without changing it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*dbPath)
			if err != nil {
				return err
			}
			cwd, _ := cmd.Flags().GetString("cwd")
			if cwd == "" {
				cwd, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			cwd, _ = filepath.Abs(cwd)
			database, err := store.InspectDatabase(cfg.DBPath)
			if err != nil {
				return err
			}
			contextDTO := output.Context{Database: output.Database{Path: cfg.DBPath, Status: string(database.Status)}, CWD: cwd, DefaultReadScopes: []output.ContextScopeDTO{}}
			warnings := managedSkillWarnings()

			observation, observeErr := repoctx.Observe(cmd.Context(), cwd)
			switch {
			case errors.Is(observeErr, repoctx.ErrOutsideRepository):
				contextDTO.Resolution = &output.ResolutionDTO{Source: "git", Status: string(store.ResolutionOutside)}
				user := logicalContextScope(domain.Scope{ID: "user", Kind: domain.ScopeKindUser, Label: "user"}, database.Status)
				contextDTO.DefaultReadScopes = append(contextDTO.DefaultReadScopes, user)
			case errors.Is(observeErr, repoctx.ErrAmbiguous):
				contextDTO.Resolution = &output.ResolutionDTO{Source: "remote", Status: string(store.ResolutionAmbiguous)}
				warnings = append(warnings, output.Warning{Code: "repository_identity_ambiguous", Message: "Configure thr.identityRemote to choose the repository identity remote."})
			case observeErr == nil && observation.IdentityAmbiguous && database.Status != store.DatabaseCompatible:
				contextDTO.Resolution = &output.ResolutionDTO{Source: "remote", Status: string(store.ResolutionAmbiguous)}
				warnings = append(warnings, output.Warning{Code: "repository_identity_ambiguous", Message: "Configure thr.identityRemote to choose the repository identity remote."})
			case observeErr != nil:
				contextDTO.Resolution = &output.ResolutionDTO{Source: "git", Status: string(store.ResolutionUnavailable)}
			case database.Status == store.DatabaseCompatible:
				db, err := store.OpenCompatibleReadOnly(cfg.DBPath)
				if err != nil {
					return err
				}
				repo := store.NewRepository(db)
				resolution, resolveErr := repo.ResolveRepository(cmd.Context(), observation)
				_ = db.Close()
				if resolveErr != nil && !errors.Is(resolveErr, store.ErrRepositoryAmbiguous) {
					return resolveErr
				}
				contextDTO.Resolution = &output.ResolutionDTO{Source: resolutionSource(observation, resolution), Status: string(resolution.Status)}
				if resolution.Status == store.ResolutionAmbiguous {
					warnings = append(warnings, output.Warning{Code: "repository_identity_ambiguous", Message: "Configure thr.identityRemote or bind this checkout explicitly."})
					break
				}
				if resolution.Scope != nil {
					current := persistedContextScope(*resolution.Scope)
					contextDTO.CurrentScope, contextDTO.DefaultWriteScope = &current, &current
					contextDTO.DefaultReadScopes = append(contextDTO.DefaultReadScopes, current)
				} else {
					prospective := prospectiveContextScope(observation.Label)
					contextDTO.ProspectiveScope, contextDTO.DefaultWriteScope = &prospective, &prospective
				}
				contextDTO.DefaultReadScopes = append(contextDTO.DefaultReadScopes, logicalContextScope(domain.Scope{ID: "user", Kind: domain.ScopeKindUser, Label: "user"}, database.Status))
				if resolution.Drift {
					warnings = append(warnings, output.Warning{Code: "repository_identity_drift", Message: "The bound checkout remote differs from its stored identity."})
				}
			default:
				prospective := prospectiveContextScope(observation.Label)
				contextDTO.ProspectiveScope, contextDTO.DefaultWriteScope = &prospective, &prospective
				contextDTO.DefaultReadScopes = append(contextDTO.DefaultReadScopes, logicalContextScope(domain.Scope{ID: "user", Kind: domain.ScopeKindUser, Label: "user"}, database.Status))
				contextDTO.Resolution = &output.ResolutionDTO{Source: resolutionSource(observation, store.RepositoryResolution{}), Status: string(store.ResolutionProspective)}
			}

			if isJSONV2Output(cmd) {
				return output.EncodeJSONV2(cmd.OutOrStdout(), output.Envelope{OK: true, Command: "context.show", Context: contextDTO, Result: map[string]any{}, Warnings: warnings})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Database: %s\n", output.SanitizeInline(cfg.DBPath))
			fmt.Fprintf(cmd.OutOrStdout(), "Database status: %s\n", database.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "Working directory: %s\n", output.SanitizeInline(cwd))
			if contextDTO.Resolution != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Repository resolution: %s (source: %s)\n", contextDTO.Resolution.Status, contextDTO.Resolution.Source)
			}
			if contextDTO.CurrentScope != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Current scope: %s\n", describeContextScope(*contextDTO.CurrentScope))
			} else if contextDTO.ProspectiveScope != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Prospective scope: %s\n", describeContextScope(*contextDTO.ProspectiveScope))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Current scope: none")
			}
			if contextDTO.DefaultWriteScope != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Default write scope: %s\n", describeContextScope(*contextDTO.DefaultWriteScope))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Default write scope: none")
			}
			readScopes := make([]string, len(contextDTO.DefaultReadScopes))
			for i, scope := range contextDTO.DefaultReadScopes {
				readScopes[i] = describeContextScope(scope)
			}
			readScopeText := strings.Join(readScopes, ", ")
			if readScopeText == "" {
				readScopeText = "none"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Default read scopes: %s\n", readScopeText)
			printWarnings(cmd, warnings)
			return nil
		},
	}
}

func describeContextScope(scope output.ContextScopeDTO) string {
	label := output.SanitizeInline(scope.Label)
	if scope.ID == nil {
		return label + " (prospective)"
	}
	id := output.SanitizeInline(*scope.ID)
	if label == "" || label == id {
		return id
	}
	return fmt.Sprintf("%s (%s)", id, label)
}

func persistedContextScope(scope domain.Scope) output.ContextScopeDTO {
	id := scope.ID
	return output.ContextScopeDTO{ID: &id, Kind: string(scope.Kind), Label: scope.Label, Status: "persisted"}
}

func logicalContextScope(scope domain.Scope, status store.DatabaseStatus) output.ContextScopeDTO {
	id := scope.ID
	state := "logical"
	if status == store.DatabaseCompatible {
		state = "persisted"
	}
	return output.ContextScopeDTO{ID: &id, Kind: string(scope.Kind), Label: scope.Label, Status: state}
}

func prospectiveContextScope(label string) output.ContextScopeDTO {
	return output.ContextScopeDTO{ID: nil, Kind: string(domain.ScopeKindRepo), Label: label, Status: "prospective"}
}

func resolutionSource(observation repoctx.Observation, resolution store.RepositoryResolution) string {
	if resolution.Status == store.ResolutionBound {
		return "git_common_dir"
	}
	if observation.CanonicalRemote != "" {
		return "canonical_origin"
	}
	return "git_common_dir"
}
