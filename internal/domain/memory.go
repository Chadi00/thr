package domain

import "time"

type Memory struct {
	ID              int64
	Text            string
	Scope           Scope
	ScopeAssignment ScopeAssignment
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ScopeUpdatedAt  time.Time
	Revision        int64
}

type ScopeKind string

const (
	ScopeKindUser ScopeKind = "user"
	ScopeKindRepo ScopeKind = "repo"
)

type ScopeAssignment string

const (
	ScopeAssignmentAutomatic ScopeAssignment = "automatic_context"
	ScopeAssignmentExplicit  ScopeAssignment = "explicit"
	ScopeAssignmentLegacy    ScopeAssignment = "legacy_default"
)

type Scope struct {
	ID    string
	Kind  ScopeKind
	Label string
}
