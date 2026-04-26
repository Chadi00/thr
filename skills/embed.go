package skills

import _ "embed"

// ThrSkill is the canonical Agent Skill installed by `thr setup`.
//
//go:embed thr/SKILL.md
var ThrSkill string
