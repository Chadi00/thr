package skills

import (
	"regexp"
	"strings"
	"testing"
)

func TestThrSkillFrontmatterIsPortable(t *testing.T) {
	if !strings.HasPrefix(ThrSkill, "---\n") {
		t.Fatal("skill must start with YAML frontmatter")
	}

	frontmatter, body, ok := strings.Cut(strings.TrimPrefix(ThrSkill, "---\n"), "\n---\n")
	if !ok {
		t.Fatal("skill must close YAML frontmatter with a standalone --- line")
	}
	if strings.TrimSpace(body) == "" {
		t.Fatal("skill body must not be empty")
	}

	fields := make(map[string]string)
	for _, line := range strings.Split(frontmatter, "\n") {
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("frontmatter line must be a simple key-value scalar: %q", line)
		}
		if strings.TrimSpace(key) != key || strings.TrimSpace(value) != value {
			t.Fatalf("frontmatter line must not use leading or trailing whitespace: %q", line)
		}
		if strings.Contains(value, ": ") {
			t.Fatalf("frontmatter scalar should avoid unquoted colon-space for parser portability: %q", line)
		}
		fields[key] = value
	}

	name := fields["name"]
	if name != "thr" {
		t.Fatalf("skill name must match the thr directory, got %q", name)
	}
	if !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(name) {
		t.Fatalf("skill name does not match Agent Skills naming rules: %q", name)
	}

	description := fields["description"]
	if description == "" {
		t.Fatal("skill description is required")
	}
	if len(description) > 1024 {
		t.Fatalf("skill description is too long: %d bytes", len(description))
	}
}

func TestThrSkillDocumentsScopeContract(t *testing.T) {
	for _, want := range []string{
		"<!-- thr:managed-skill:v2 -->",
		"default recall searches that repository plus the user scope",
		"least-broad scope",
		"--scope user",
		"thr --format json-v2 context",
		"thr --format json-v2 ask",
		"--cwd",
		"thr move",
		"repo:<id>",
	} {
		if !strings.Contains(ThrSkill, want) {
			t.Fatalf("skill must document %q", want)
		}
	}
	if strings.Contains(ThrSkill, "<!-- thr:managed-skill:v1 -->") {
		t.Fatal("bundled skill must use only the v2 managed marker")
	}
}
