package repoctx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const canonicalizationVersion = 1

var (
	ErrOutsideRepository = errors.New("outside repository")
	ErrUnavailable       = errors.New("repository context unavailable")
	ErrAmbiguous         = errors.New("repository identity ambiguous")

	scpRemote = regexp.MustCompile(`^(?:[^/@:]+@)?([^/:]+):(.+)$`)
)

type Observation struct {
	WorktreeRoot            string
	CommonDir               string
	Bare                    bool
	RemoteName              string
	RemoteURL               string
	CanonicalRemote         string
	CanonicalizationVersion int
	Label                   string
	IdentityAmbiguous       bool
}

func Observe(ctx context.Context, cwd string) (Observation, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return Observation{}, fmt.Errorf("%w: resolve cwd", ErrUnavailable)
		}
	}
	absCWD, err := filepath.Abs(cwd)
	if err != nil {
		return Observation{}, fmt.Errorf("%w: resolve cwd", ErrUnavailable)
	}

	common, err := git(ctx, absCWD, "resolve repository", "rev-parse", "--git-common-dir")
	if err != nil {
		if isOutsideRepository(err) {
			return Observation{}, ErrOutsideRepository
		}
		return Observation{}, unavailable(ctx, "resolve repository")
	}
	commonDir, err := canonicalPath(absCWD, common)
	if err != nil {
		return Observation{}, fmt.Errorf("%w: resolve Git common directory", ErrUnavailable)
	}

	bareText, err := git(ctx, absCWD, "detect bare repository", "rev-parse", "--is-bare-repository")
	if err != nil {
		return Observation{}, unavailable(ctx, "detect bare repository")
	}
	bare := bareText == "true"
	observation := Observation{CommonDir: commonDir, Bare: bare}
	if !bare {
		root, err := git(ctx, absCWD, "resolve worktree root", "rev-parse", "--show-toplevel")
		if err != nil {
			return Observation{}, unavailable(ctx, "resolve worktree root")
		}
		observation.WorktreeRoot, err = canonicalPath(absCWD, root)
		if err != nil {
			return Observation{}, fmt.Errorf("%w: resolve worktree root", ErrUnavailable)
		}
	}

	remoteName, remoteURL, err := identityRemote(ctx, absCWD)
	if err != nil {
		if errors.Is(err, ErrAmbiguous) {
			observation.IdentityAmbiguous = true
		} else {
			return Observation{}, err
		}
	}
	observation.RemoteName, observation.RemoteURL = remoteName, remoteURL
	if remoteURL != "" {
		observation.CanonicalRemote, err = canonicalizeRemote(remoteURL)
		if err != nil {
			return Observation{}, fmt.Errorf("%w: canonicalize identity remote", ErrUnavailable)
		}
		observation.CanonicalizationVersion = canonicalizationVersion
	}
	observation.Label = label(observation)
	return observation, nil
}

func identityRemote(ctx context.Context, cwd string) (string, string, error) {
	configured, err := gitOptional(ctx, cwd, "read identity remote", "config", "--local", "--get", "thr.identityRemote")
	if err != nil {
		return "", "", unavailable(ctx, "read identity remote")
	}
	remotesText, err := git(ctx, cwd, "list remotes", "remote")
	if err != nil {
		return "", "", unavailable(ctx, "list remotes")
	}

	remotes := make(map[string]string)
	for _, name := range strings.Fields(remotesText) {
		value, err := gitOptional(ctx, cwd, "read remote", "config", "--local", "--get", "remote."+name+".url")
		if err != nil {
			return "", "", unavailable(ctx, "read remote")
		}
		if value != "" {
			remotes[name] = value
		}
	}

	selected := ""
	switch {
	case configured != "":
		selected = configured
		if remotes[selected] == "" {
			return "", "", fmt.Errorf("%w: configured identity remote has no fetch URL", ErrUnavailable)
		}
	case remotes["origin"] != "":
		selected = "origin"
	case len(remotes) == 1:
		for name := range remotes {
			selected = name
		}
	case len(remotes) > 1:
		return "", "", ErrAmbiguous
	default:
		return "", "", nil
	}

	sanitized, err := sanitizeRemote(remotes[selected])
	if err != nil {
		return "", "", fmt.Errorf("%w: sanitize identity remote", ErrUnavailable)
	}
	return selected, sanitized, nil
}

type gitError struct {
	outside  bool
	exitCode int
}

func (e *gitError) Error() string { return "git command failed" }

func git(ctx context.Context, cwd, operation string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", cwd}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := string(output)
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return "", &gitError{
			outside:  operation == "resolve repository" && strings.Contains(text, "not a git repository"),
			exitCode: exitCode,
		}
	}
	return strings.TrimSpace(string(output)), nil
}

func gitOptional(ctx context.Context, cwd, operation string, args ...string) (string, error) {
	value, err := git(ctx, cwd, operation, args...)
	if err == nil {
		return value, nil
	}
	var gitErr *gitError
	if errors.As(err, &gitErr) && gitErr.exitCode == 1 {
		return "", nil
	}
	return "", err
}

func unavailable(ctx context.Context, operation string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrUnavailable, operation, err)
	}
	return fmt.Errorf("%w: %s", ErrUnavailable, operation)
}

func isOutsideRepository(err error) bool {
	var gitErr *gitError
	return errors.As(err, &gitErr) && gitErr.outside
}

func canonicalPath(cwd, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}

func canonicalizeRemote(remote string) (string, error) {
	remote, err := sanitizeRemote(remote)
	if err != nil {
		return "", err
	}
	if parsed, err := url.Parse(remote); err == nil && strings.Contains(remote, "://") {
		host := strings.ToLower(parsed.Hostname())
		if knownProvider(host) && (parsed.Scheme == "https" || parsed.Scheme == "ssh") && parsed.RawQuery == "" && parsed.Fragment == "" {
			port := parsed.Port()
			if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "ssh" && port == "22") {
				port = ""
			}
			return providerRemote(host, port, strings.TrimPrefix(parsed.EscapedPath(), "/")), nil
		}
		return remote, nil
	}
	if parts := scpRemote.FindStringSubmatch(remote); parts != nil && knownProvider(parts[1]) {
		return providerRemote(parts[1], "", parts[2]), nil
	}
	return remote, nil
}

func sanitizeRemote(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", errors.New("empty remote")
	}
	if strings.Contains(remote, "://") {
		parsed, err := url.Parse(remote)
		if err != nil || parsed.Host == "" {
			return "", errors.New("invalid remote URL")
		}
		parsed.User = nil
		parsed.Host = normalizedHost(parsed.Hostname(), parsed.Port())
		return parsed.String(), nil
	}
	if parts := scpRemote.FindStringSubmatch(remote); parts != nil {
		return strings.ToLower(parts[1]) + ":" + parts[2], nil
	}
	return remote, nil
}

func normalizedHost(host, port string) string {
	host = strings.ToLower(host)
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

func knownProvider(host string) bool {
	switch strings.ToLower(host) {
	case "github.com", "gitlab.com", "bitbucket.org":
		return true
	default:
		return false
	}
}

func providerRemote(host, port, path string) string {
	path = strings.TrimSuffix(path, ".git")
	if port != "" {
		host = net.JoinHostPort(host, port)
	}
	return host + "/" + path
}

func label(observation Observation) string {
	if observation.CanonicalRemote != "" {
		if parsed, err := url.Parse(observation.CanonicalRemote); err == nil && parsed.Host != "" {
			return parsed.Host + parsed.Path
		}
		return strings.Replace(observation.CanonicalRemote, ":", "/", 1)
	}
	path := observation.WorktreeRoot
	if observation.Bare {
		path = observation.CommonDir
	}
	return strings.TrimSuffix(filepath.Base(path), ".git")
}
