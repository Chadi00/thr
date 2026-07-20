package repoctx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalizeRemote(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{"GitHub scp", "git@GitHub.com:Acme/Repo.git", "github.com/Acme/Repo"},
		{"GitHub SSH", "ssh://git@GitHub.com/Acme/Repo.git", "github.com/Acme/Repo"},
		{"GitLab HTTPS credentials", "https://token:secret@GitLab.com/Group/Repo.git", "gitlab.com/Group/Repo"},
		{"Bitbucket default SSH port", "ssh://git@bitbucket.org:22/Team/Repo.git", "bitbucket.org/Team/Repo"},
		{"non-default HTTPS port", "https://gitlab.com:8443/Group/Repo.git", "gitlab.com:8443/Group/Repo"},
		{"path case", "https://github.com/OWNER/MixedCase.git", "github.com/OWNER/MixedCase"},
		{"unknown HTTPS", "https://user:pass@Example.com/Acme/Repo.git", "https://example.com/Acme/Repo.git"},
		{"unknown scp", "deploy@Example.com:Acme/Repo.git", "example.com:Acme/Repo.git"},
		{"local path", "/srv/git/Repo.git", "/srv/git/Repo.git"},
		{"known provider query remains conservative", "https://GitHub.com/Acme/Repo.git?ref=x", "https://github.com/Acme/Repo.git?ref=x"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := canonicalizeRemote(test.remote)
			if err != nil {
				t.Fatalf("canonicalize remote: %v", err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestObserveRepositoryShapes(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	base := t.TempDir()
	canonicalBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatal(err)
	}
	outer := filepath.Join(base, "outer")
	inner := filepath.Join(outer, "nested", "inner")
	runGit(t, base, "init", outer)
	runGit(t, outer, "init", inner)
	runGit(t, inner, "config", "--local", "remote.publish.pushurl", "https://example.com/Acme/Publish.git")
	subdir := filepath.Join(inner, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	observation, err := Observe(ctx, subdir)
	if err != nil {
		t.Fatalf("observe nested repository: %v", err)
	}
	after, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("process cwd changed from %q to %q", before, after)
	}
	wantInner := filepath.Join(canonicalBase, "outer", "nested", "inner")
	if observation.WorktreeRoot != wantInner {
		t.Fatalf("worktree root: got %q want %q", observation.WorktreeRoot, wantInner)
	}
	if observation.CommonDir != filepath.Join(wantInner, ".git") || observation.Bare {
		t.Fatalf("unexpected nested observation: %+v", observation)
	}
	if observation.RemoteURL != "" || observation.Label != "inner" {
		t.Fatalf("unexpected local evidence: %+v", observation)
	}

	bare := filepath.Join(base, "archive.git")
	runGit(t, base, "init", "--bare", bare)
	observation, err = Observe(ctx, bare)
	if err != nil {
		t.Fatalf("observe bare repository: %v", err)
	}
	wantBare := filepath.Join(canonicalBase, "archive.git")
	if !observation.Bare || observation.WorktreeRoot != "" || observation.CommonDir != wantBare || observation.Label != "archive" {
		t.Fatalf("unexpected bare observation: %+v", observation)
	}
}

func TestObserveLinkedWorktreeUsesCommonDirectory(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	base := t.TempDir()
	main := filepath.Join(base, "main")
	linked := filepath.Join(base, "linked")
	runGit(t, base, "init", main)
	runGit(t, main, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial")
	runGit(t, main, "worktree", "add", "--detach", linked)

	mainObservation, err := Observe(ctx, main)
	if err != nil {
		t.Fatalf("observe main worktree: %v", err)
	}
	linkedObservation, err := Observe(ctx, linked)
	if err != nil {
		t.Fatalf("observe linked worktree: %v", err)
	}
	if mainObservation.CommonDir != linkedObservation.CommonDir {
		t.Fatalf("common dirs differ: %q and %q", mainObservation.CommonDir, linkedObservation.CommonDir)
	}
	if mainObservation.WorktreeRoot == linkedObservation.WorktreeRoot {
		t.Fatalf("worktree roots should differ: %q", mainObservation.WorktreeRoot)
	}
}

func TestObserveRemoteSelectionAndSanitization(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	runGit(t, base, "init", repo)
	runGit(t, repo, "remote", "add", "fork", "https://user:secret@GitHub.com/Acme/Fork.git")

	observation, err := Observe(ctx, repo)
	if err != nil {
		t.Fatalf("observe sole remote: %v", err)
	}
	if observation.RemoteName != "fork" || observation.RemoteURL != "https://github.com/Acme/Fork.git" || observation.CanonicalRemote != "github.com/Acme/Fork" || observation.CanonicalizationVersion != 1 || observation.Label != "github.com/Acme/Fork" {
		t.Fatalf("unexpected sole remote observation: %+v", observation)
	}
	if strings.Contains(observation.RemoteURL, "secret") || strings.Contains(observation.RemoteURL, "user") {
		t.Fatalf("credentials escaped in observation: %+v", observation)
	}

	runGit(t, repo, "remote", "add", "upstream", "https://gitlab.com/Acme/Upstream.git")
	if observation, err := Observe(ctx, repo); err != nil || !observation.IdentityAmbiguous {
		t.Fatalf("expected ambiguity observation, got %+v, %v", observation, err)
	}

	runGit(t, repo, "config", "--local", "thr.identityRemote", "https://user:secret@example.com/repo.git")
	if _, err := Observe(ctx, repo); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("expected sanitized unavailable error, got %v", err)
	}

	runGit(t, repo, "config", "--local", "thr.identityRemote", "upstream")
	observation, err = Observe(ctx, repo)
	if err != nil {
		t.Fatalf("observe configured identity remote: %v", err)
	}
	if observation.RemoteName != "upstream" || observation.CanonicalRemote != "gitlab.com/Acme/Upstream" {
		t.Fatalf("configured remote not selected: %+v", observation)
	}

	runGit(t, repo, "config", "--local", "--unset", "thr.identityRemote")
	runGit(t, repo, "remote", "rename", "fork", "origin")
	observation, err = Observe(ctx, repo)
	if err != nil {
		t.Fatalf("observe origin: %v", err)
	}
	if observation.RemoteName != "origin" || observation.CanonicalRemote != "github.com/Acme/Fork" {
		t.Fatalf("origin not selected: %+v", observation)
	}
}

func TestObserveErrors(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	outside := t.TempDir()
	if _, err := Observe(ctx, outside); !errors.Is(err, ErrOutsideRepository) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected outside repository error, got %v", err)
	}
	if _, err := Observe(ctx, filepath.Join(outside, "missing")); !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrOutsideRepository) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	t.Setenv("PATH", "")
	if _, err := Observe(ctx, outside); !errors.Is(err, ErrUnavailable) || errors.Is(err, ErrOutsideRepository) {
		t.Fatalf("expected unavailable Git error, got %v", err)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
}

func runGit(t *testing.T, cwd string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
