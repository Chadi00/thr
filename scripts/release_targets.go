package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultLockPath = "native/onnxruntime.lock"

type runtimeLock struct {
	SchemaVersion      int             `json:"schema_version"`
	ONNXRuntimeVersion string          `json:"onnxruntime_version"`
	NativeReleaseTag   string          `json:"native_release_tag"`
	RuntimePolicy      json.RawMessage `json:"runtime_policy,omitempty"`
	Targets            []runtimeTarget `json:"targets"`
}

type runtimeTarget struct {
	Target               string   `json:"target"`
	Status               string   `json:"status"`
	OS                   string   `json:"os"`
	Arch                 string   `json:"arch"`
	Runner               string   `json:"runner"`
	Installer            string   `json:"installer"`
	Source               string   `json:"source"`
	SourceURL            string   `json:"source_url,omitempty"`
	SourceArchiveSHA256  string   `json:"source_archive_sha256,omitempty"`
	SourceRepo           string   `json:"source_repo,omitempty"`
	SourceTag            string   `json:"source_tag,omitempty"`
	SourceLibraryPath    string   `json:"source_library_path,omitempty"`
	RuntimeAssetName     string   `json:"runtime_asset_name,omitempty"`
	RuntimeAssetURL      string   `json:"runtime_asset_url,omitempty"`
	RuntimeArchiveSHA256 string   `json:"runtime_archive_sha256,omitempty"`
	RuntimeLibraryPath   string   `json:"runtime_library_path,omitempty"`
	RuntimeLibrarySHA256 string   `json:"runtime_library_sha256,omitempty"`
	LicenseFiles         []string `json:"license_files,omitempty"`
}

type matrix struct {
	Include []map[string]string `json:"include"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: release_targets.go <release-matrix|smoke-matrix|native-matrix|native-release-tag|env|native-env|update-lock|validate> [flags]")
	}

	switch args[0] {
	case "release-matrix":
		fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
		lockPath := fs.String("lock", defaultLockPath, "runtime lock path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		lock, err := readLock(*lockPath)
		if err != nil {
			return err
		}
		return writeJSON(buildMatrix(lock, func(t runtimeTarget) bool {
			return t.Status == "shipping"
		}))
	case "smoke-matrix":
		fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
		lockPath := fs.String("lock", defaultLockPath, "runtime lock path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		lock, err := readLock(*lockPath)
		if err != nil {
			return err
		}
		return writeJSON(buildMatrix(lock, func(t runtimeTarget) bool {
			return t.Status == "shipping" && t.Installer == "unix"
		}))
	case "native-matrix":
		fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
		lockPath := fs.String("lock", defaultLockPath, "runtime lock path")
		onlyTarget := fs.String("target", "all", "target to include, or all")
		missingRelease := fs.Bool("missing-release", false, "only include shipping targets missing release runtime pins")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		lock, err := readLock(*lockPath)
		if err != nil {
			return err
		}
		matrix := buildMatrix(lock, func(t runtimeTarget) bool {
			return includeNativeTarget(lock, t, *onlyTarget, *missingRelease)
		})
		if *onlyTarget != "all" && len(matrix.Include) == 0 {
			return fmt.Errorf("target %q is not a shipping native runtime target", *onlyTarget)
		}
		return writeJSON(matrix)
	case "native-release-tag":
		fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
		lockPath := fs.String("lock", defaultLockPath, "runtime lock path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		lock, err := readLock(*lockPath)
		if err != nil {
			return err
		}
		fmt.Println(lock.NativeReleaseTag)
		return nil
	case "env":
		return writeTargetEnv(args[1:], false)
	case "native-env":
		return writeTargetEnv(args[1:], true)
	case "update-lock":
		fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
		lockPath := fs.String("lock", defaultLockPath, "runtime lock path")
		artifactDir := fs.String("artifact-dir", "dist-native", "directory containing native runtime artifacts and checksums")
		repo := fs.String("repo", "", "GitHub repository in owner/name form for runtime asset URLs")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		lock, err := readLock(*lockPath)
		if err != nil {
			return err
		}
		updated, count, err := updateLockFromArtifacts(lock, *artifactDir, *repo)
		if err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("no native runtime artifacts in %s matched shipping targets in %s", *artifactDir, *lockPath)
		}
		if err := validateLock(updated, "metadata"); err != nil {
			return err
		}
		return writeLockFile(*lockPath, updated)
	case "validate":
		fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
		lockPath := fs.String("lock", defaultLockPath, "runtime lock path")
		mode := fs.String("mode", "metadata", "metadata or release")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		lock, err := readLock(*lockPath)
		if err != nil {
			return err
		}
		return validateLock(lock, *mode)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func writeTargetEnv(args []string, native bool) error {
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	lockPath := fs.String("lock", defaultLockPath, "runtime lock path")
	targetName := fs.String("target", "", "target name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *targetName == "" {
		return errors.New("--target is required")
	}

	lock, err := readLock(*lockPath)
	if err != nil {
		return err
	}
	target, ok := findTarget(lock, *targetName)
	if !ok {
		return fmt.Errorf("target %q is not present in %s", *targetName, *lockPath)
	}
	if target.Status != "shipping" {
		return fmt.Errorf("target %q is %q, not shipping", target.Target, target.Status)
	}
	if native {
		if err := validateNativeTarget(target); err != nil {
			return err
		}
	} else if err := validateReleaseTarget(lock, target); err != nil {
		return err
	}

	values := map[string]string{
		"THR_TARGET":                 target.Target,
		"THR_TARGET_OS":              target.OS,
		"THR_TARGET_ARCH":            target.Arch,
		"THR_TARGET_RUNNER":          target.Runner,
		"THR_ONNXRUNTIME_VERSION":    lock.ONNXRuntimeVersion,
		"THR_RUNTIME_SOURCE":         target.Source,
		"THR_RUNTIME_ASSET_NAME":     target.RuntimeAssetName,
		"THR_RUNTIME_LIBRARY_PATH":   target.RuntimeLibraryPath,
		"THR_RUNTIME_LIBRARY_SHA256": target.RuntimeLibrarySHA256,
	}
	if native {
		values["THR_NATIVE_RELEASE_TAG"] = lock.NativeReleaseTag
		values["THR_SOURCE_URL"] = target.SourceURL
		values["THR_SOURCE_ARCHIVE_SHA256"] = target.SourceArchiveSHA256
		values["THR_SOURCE_REPO"] = target.SourceRepo
		values["THR_SOURCE_TAG"] = target.SourceTag
		values["THR_SOURCE_LIBRARY_PATH"] = target.SourceLibraryPath
	} else {
		values["THR_RUNTIME_ASSET_URL"] = target.RuntimeAssetURL
		values["THR_RUNTIME_ARCHIVE_SHA256"] = target.RuntimeArchiveSHA256
	}
	for key, value := range values {
		fmt.Printf("%s=%s\n", key, shellQuote(value))
	}
	if len(target.LicenseFiles) > 0 {
		fmt.Printf("THR_RUNTIME_LICENSE_FILES=%s\n", shellQuote(strings.Join(target.LicenseFiles, " ")))
	}
	return nil
}

func readLock(path string) (runtimeLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return runtimeLock{}, err
	}
	var lock runtimeLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return runtimeLock{}, err
	}
	return lock, validateLock(lock, "metadata")
}

func writeLockFile(path string, lock runtimeLock) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func findTarget(lock runtimeLock, name string) (runtimeTarget, bool) {
	for _, target := range lock.Targets {
		if target.Target == name {
			return target, true
		}
	}
	return runtimeTarget{}, false
}

func buildMatrix(lock runtimeLock, include func(runtimeTarget) bool) matrix {
	out := matrix{Include: []map[string]string{}}
	for _, target := range lock.Targets {
		if !include(target) {
			continue
		}
		out.Include = append(out.Include, map[string]string{
			"target":        target.Target,
			"goos":          target.OS,
			"goarch":        target.Arch,
			"runner":        target.Runner,
			"installer":     target.Installer,
			"archive":       fmt.Sprintf("thr_%s_%s.tar.gz", target.OS, target.Arch),
			"runtimeAsset":  target.RuntimeAssetName,
			"runtimeSource": target.Source,
		})
	}
	sort.Slice(out.Include, func(i, j int) bool {
		return out.Include[i]["target"] < out.Include[j]["target"]
	})
	return out
}

func includeNativeTarget(lock runtimeLock, target runtimeTarget, onlyTarget string, missingRelease bool) bool {
	if target.Status != "shipping" {
		return false
	}
	if onlyTarget != "all" && target.Target != onlyTarget {
		return false
	}
	return !missingRelease || !releaseTargetReady(lock, target)
}

func validateLock(lock runtimeLock, mode string) error {
	if lock.SchemaVersion != 2 {
		return fmt.Errorf("native runtime lock schema_version must be 2, got %d", lock.SchemaVersion)
	}
	if lock.ONNXRuntimeVersion == "" {
		return errors.New("native runtime lock is missing onnxruntime_version")
	}
	if lock.NativeReleaseTag == "" {
		return errors.New("native runtime lock is missing native_release_tag")
	}
	if len(lock.Targets) == 0 {
		return errors.New("native runtime lock must contain at least one target")
	}

	seen := map[string]bool{}
	for _, target := range lock.Targets {
		if target.Target == "" {
			return errors.New("runtime target is missing target")
		}
		if seen[target.Target] {
			return fmt.Errorf("duplicate runtime target %q", target.Target)
		}
		seen[target.Target] = true
		if target.Status == "" || target.OS == "" || target.Arch == "" || target.Runner == "" || target.Source == "" {
			return fmt.Errorf("runtime target %q is missing required metadata", target.Target)
		}
		if target.Target != target.OS+"-"+target.Arch {
			return fmt.Errorf("runtime target %q does not match os/arch %s-%s", target.Target, target.OS, target.Arch)
		}
		if target.Status == "shipping" {
			if err := validateNativeTarget(target); err != nil {
				return err
			}
			if mode == "release" {
				if err := validateReleaseTarget(lock, target); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateNativeTarget(target runtimeTarget) error {
	if target.RuntimeAssetName == "" || target.RuntimeLibraryPath == "" || len(target.LicenseFiles) == 0 {
		return fmt.Errorf("shipping target %q is missing normalized runtime artifact metadata", target.Target)
	}
	switch target.Source {
	case "official-release-asset":
		if target.SourceURL == "" || target.SourceArchiveSHA256 == "" || target.SourceLibraryPath == "" {
			return fmt.Errorf("shipping target %q is missing official source asset metadata", target.Target)
		}
	case "source-build":
		if target.SourceRepo == "" || target.SourceTag == "" || target.SourceLibraryPath == "" {
			return fmt.Errorf("shipping target %q is missing source-build metadata", target.Target)
		}
	default:
		return fmt.Errorf("shipping target %q has unsupported runtime source %q", target.Target, target.Source)
	}
	return nil
}

func validateReleaseTarget(lock runtimeLock, target runtimeTarget) error {
	if target.RuntimeAssetURL == "" || target.RuntimeArchiveSHA256 == "" || target.RuntimeLibrarySHA256 == "" {
		return fmt.Errorf("shipping target %q is not release-ready; run native-runtime and update native/onnxruntime.lock with runtime URL and SHA-256 values", target.Target)
	}
	if !strings.HasPrefix(target.RuntimeAssetURL, "file://") && !strings.HasSuffix(target.RuntimeAssetURL, expectedRuntimeAssetURLSuffix(lock, target)) {
		return fmt.Errorf("shipping target %q runtime_asset_url does not match native release tag and asset name", target.Target)
	}
	return nil
}

func releaseTargetReady(lock runtimeLock, target runtimeTarget) bool {
	return validateReleaseTarget(lock, target) == nil
}

func expectedRuntimeAssetURL(repo string, lock runtimeLock, target runtimeTarget) string {
	return fmt.Sprintf("https://github.com/%s%s", strings.Trim(repo, "/"), expectedRuntimeAssetURLSuffix(lock, target))
}

func expectedRuntimeAssetURLSuffix(lock runtimeLock, target runtimeTarget) string {
	return fmt.Sprintf("/releases/download/%s/%s", lock.NativeReleaseTag, target.RuntimeAssetName)
}

func updateLockFromArtifacts(lock runtimeLock, artifactDir string, repo string) (runtimeLock, int, error) {
	archiveSHAs, err := readArchiveChecksums(artifactDir)
	if err != nil {
		return runtimeLock{}, 0, err
	}
	librarySHAs, err := readLibraryChecksums(artifactDir)
	if err != nil {
		return runtimeLock{}, 0, err
	}

	updates := 0
	for i := range lock.Targets {
		target := &lock.Targets[i]
		if target.Status != "shipping" || target.RuntimeAssetName == "" {
			continue
		}
		assetName := target.RuntimeAssetName
		targetUpdated := false
		if sha, ok := archiveSHAs[assetName]; ok {
			target.RuntimeArchiveSHA256 = sha
			targetUpdated = true
		}
		if sha, ok := librarySHAs[assetName]; ok {
			target.RuntimeLibrarySHA256 = sha
			targetUpdated = true
		}
		if targetUpdated {
			if repo != "" {
				target.RuntimeAssetURL = expectedRuntimeAssetURL(repo, lock, *target)
			}
			updates++
		}
	}
	return lock, updates, nil
}

func readArchiveChecksums(artifactDir string) (map[string]string, error) {
	checksumsPath := filepath.Join(artifactDir, "checksums.txt")
	if _, err := os.Stat(checksumsPath); err == nil {
		return readChecksumFile(checksumsPath, func(path string) string {
			return filepath.Base(path)
		})
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	out := map[string]string{}
	matches, err := filepath.Glob(filepath.Join(artifactDir, "*.tar.gz.sha256"))
	if err != nil {
		return nil, err
	}
	for _, path := range matches {
		values, err := readChecksumFile(path, func(_ string) string {
			return strings.TrimSuffix(filepath.Base(path), ".sha256")
		})
		if err != nil {
			return nil, err
		}
		for name, sha := range values {
			out[name] = sha
		}
	}
	return out, nil
}

func readLibraryChecksums(artifactDir string) (map[string]string, error) {
	out := map[string]string{}
	matches, err := filepath.Glob(filepath.Join(artifactDir, "*.tar.gz.lib.sha256"))
	if err != nil {
		return nil, err
	}
	for _, path := range matches {
		values, err := readChecksumFile(path, func(_ string) string {
			return strings.TrimSuffix(filepath.Base(path), ".lib.sha256")
		})
		if err != nil {
			return nil, err
		}
		for name, sha := range values {
			out[name] = sha
		}
	}
	return out, nil
}

func readChecksumFile(path string, nameFromPath func(string) string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for lineNo, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("%s:%d: expected '<sha256> <path>'", path, lineNo+1)
		}
		out[nameFromPath(fields[1])] = fields[0]
	}
	return out, nil
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
