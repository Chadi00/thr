package output

import (
	"io"
	"strconv"
	"time"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/store"
)

const APIVersion = "thr.cli/v2"

type Envelope struct {
	APIVersion string           `json:"api_version"`
	OK         bool             `json:"ok"`
	Command    string           `json:"command"`
	Context    Context          `json:"context"`
	Result     any              `json:"result"`
	Error      *StructuredError `json:"error"`
	Warnings   []Warning        `json:"warnings"`
}

type Context struct {
	Database          Database          `json:"database"`
	CWD               string            `json:"cwd"`
	ScopeSelection    *ScopeSelection   `json:"scope_selection,omitempty"`
	CurrentScope      *ContextScopeDTO  `json:"current_scope,omitempty"`
	ProspectiveScope  *ContextScopeDTO  `json:"prospective_scope,omitempty"`
	DefaultWriteScope *ContextScopeDTO  `json:"default_write_scope,omitempty"`
	DefaultReadScopes []ContextScopeDTO `json:"default_read_scopes,omitempty"`
	Resolution        *ResolutionDTO    `json:"resolution,omitempty"`
}

type Database struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type ScopeSelection struct {
	Mode      string   `json:"mode"`
	Requested []string `json:"requested"`
	Resolved  []string `json:"resolved"`
}

type StructuredError struct {
	Code             string         `json:"code"`
	Message          string         `json:"message"`
	Retryable        bool           `json:"retryable"`
	SuggestedCommand string         `json:"suggested_command,omitempty"`
	Details          map[string]any `json:"details,omitempty"`
}

type Warning struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type ScopeDTO struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type ContextScopeDTO struct {
	ID     *string `json:"id"`
	Kind   string  `json:"kind"`
	Label  string  `json:"label"`
	Status string  `json:"status"`
}

type ResolutionDTO struct {
	Source string `json:"source"`
	Status string `json:"status"`
}

type MemoryDTO struct {
	ID              string   `json:"id"`
	Text            string   `json:"text"`
	Scope           ScopeDTO `json:"scope"`
	ScopeAssignment string   `json:"scope_assignment"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	ScopeUpdatedAt  string   `json:"scope_updated_at"`
	Revision        int64    `json:"revision"`
}

type MatchDTO struct {
	Kind     string  `json:"kind"`
	Distance float64 `json:"distance"`
}

type SemanticMatchDTO struct {
	Rank   int       `json:"rank"`
	Memory MemoryDTO `json:"memory"`
	Match  MatchDTO  `json:"match"`
}

func NewScopeDTO(scope domain.Scope) ScopeDTO {
	return ScopeDTO{ID: scope.ID, Kind: string(scope.Kind), Label: scope.Label}
}

func NewMemoryDTO(memory domain.Memory) MemoryDTO {
	return MemoryDTO{
		ID:              strconv.FormatInt(memory.ID, 10),
		Text:            memory.Text,
		Scope:           NewScopeDTO(memory.Scope),
		ScopeAssignment: string(memory.ScopeAssignment),
		CreatedAt:       formatTime(memory.CreatedAt),
		UpdatedAt:       formatTime(memory.UpdatedAt),
		ScopeUpdatedAt:  formatTime(memory.ScopeUpdatedAt),
		Revision:        memory.Revision,
	}
}

func NewSemanticMatchDTO(rank int, hit store.SemanticHit) SemanticMatchDTO {
	return SemanticMatchDTO{
		Rank: rank, Memory: NewMemoryDTO(hit.Memory),
		Match: MatchDTO{Kind: "semantic", Distance: hit.Distance},
	}
}

func EncodeJSONV2(w io.Writer, envelope Envelope) error {
	if envelope.APIVersion == "" {
		envelope.APIVersion = APIVersion
	}
	if envelope.Warnings == nil {
		envelope.Warnings = []Warning{}
	}
	if selection := envelope.Context.ScopeSelection; selection != nil {
		copy := *selection
		if copy.Requested == nil {
			copy.Requested = []string{}
		}
		if copy.Resolved == nil {
			copy.Resolved = []string{}
		}
		envelope.Context.ScopeSelection = &copy
	}
	return encodeJSON(w, envelope)
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
