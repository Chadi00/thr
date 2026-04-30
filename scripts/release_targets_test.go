package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestReleaseMatrixUsesShippingTargets(t *testing.T) {
	lock := fixtureLock()
	matrix := buildMatrix(lock, func(target runtimeTarget) bool {
		return target.Status == "shipping"
	})

	if len(matrix.Include) != 4 {
		t.Fatalf("expected four shipping targets, got %d", len(matrix.Include))
	}
	if matrix.Include[0]["target"] != "darwin-amd64" ||
		matrix.Include[1]["target"] != "darwin-arm64" ||
		matrix.Include[2]["target"] != "linux-amd64" ||
		matrix.Include[3]["target"] != "linux-arm64" {
		t.Fatalf("matrix was not target-sorted: %#v", matrix.Include)
	}
	if matrix.Include[0]["archive"] != "thr_darwin_amd64.tar.gz" {
		t.Fatalf("unexpected archive name: %s", matrix.Include[0]["archive"])
	}
	if matrix.Include[2]["archive"] != "thr_linux_amd64.tar.gz" {
		t.Fatalf("unexpected linux archive name: %s", matrix.Include[2]["archive"])
	}
}

func TestValidateReleaseRequiresPinnedRuntime(t *testing.T) {
	lock := fixtureLock()
	lock.Targets[0].RuntimeArchiveSHA256 = ""

	if err := validateLock(lock, "metadata"); err != nil {
		t.Fatalf("metadata validation should allow runtime assets before they are published: %v", err)
	}
	if err := validateLock(lock, "release"); err == nil {
		t.Fatal("expected release validation to require runtime archive SHA")
	}
}

func TestNativeMatrixCanSelectMissingRuntimePins(t *testing.T) {
	lock := fixtureLock()
	lock.Targets[0].RuntimeArchiveSHA256 = ""

	matrix := buildMatrix(lock, func(target runtimeTarget) bool {
		return includeNativeTarget(lock, target, "all", true)
	})

	if len(matrix.Include) != 1 {
		t.Fatalf("expected one target with missing pins, got %d", len(matrix.Include))
	}
	if matrix.Include[0]["target"] != "darwin-arm64" {
		t.Fatalf("unexpected missing target: %#v", matrix.Include)
	}
}

func TestValidateReleaseRejectsStaleRuntimeURL(t *testing.T) {
	lock := fixtureLock()
	lock.Targets[0].RuntimeAssetURL = "https://example.com/releases/download/old-tag/" + lock.Targets[0].RuntimeAssetName

	if err := validateLock(lock, "release"); err == nil {
		t.Fatal("expected release validation to reject stale runtime_asset_url")
	}
}

func TestUpdateLockFromArtifacts(t *testing.T) {
	lock := fixtureLock()
	lock.Targets[2].RuntimeAssetURL = ""
	lock.Targets[2].RuntimeArchiveSHA256 = ""
	lock.Targets[2].RuntimeLibrarySHA256 = ""

	artifactDir := t.TempDir()
	assetName := lock.Targets[2].RuntimeAssetName
	if err := os.WriteFile(filepath.Join(artifactDir, "checksums.txt"), []byte("archive-sha-new  "+assetName+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, assetName+".lib.sha256"), []byte("library-sha-new  lib/libonnxruntime.so\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	updated, count, err := updateLockFromArtifacts(lock, artifactDir, "Chadi00/thr")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one updated target, got %d", count)
	}

	target := updated.Targets[2]
	if target.RuntimeArchiveSHA256 != "archive-sha-new" {
		t.Fatalf("archive sha = %q", target.RuntimeArchiveSHA256)
	}
	if target.RuntimeLibrarySHA256 != "library-sha-new" {
		t.Fatalf("library sha = %q", target.RuntimeLibrarySHA256)
	}
	wantURL := "https://github.com/Chadi00/thr/releases/download/thr-native-onnxruntime-v1.25.1/" + assetName
	if target.RuntimeAssetURL != wantURL {
		t.Fatalf("runtime URL = %q, want %q", target.RuntimeAssetURL, wantURL)
	}
	if err := validateLock(updated, "release"); err != nil {
		t.Fatalf("updated lock should be release-ready: %v", err)
	}
}

func TestReadLockRejectsDuplicateTargets(t *testing.T) {
	lock := fixtureLock()
	lock.Targets = append(lock.Targets, lock.Targets[0])
	path := writeLock(t, lock)

	if _, err := readLock(path); err == nil {
		t.Fatal("expected duplicate targets to fail validation")
	}
}

func TestONNXRuntimeVersionConsistency(t *testing.T) {
	lock, err := readLock(filepath.Join("..", "native", "onnxruntime.lock"))
	if err != nil {
		t.Fatal(err)
	}

	checks := map[string]string{
		"install.sh":                    extractVersion(t, filepath.Join("..", "install.sh"), `(?m)^THR_ONNXRUNTIME_VERSION="([^"]+)"`),
		"scripts/package_release.sh":    extractVersion(t, "package_release.sh", `(?m)^ONNXRUNTIME_VERSION="([^"]+)"`),
		"internal/embed/onnxruntime.go": extractVersion(t, filepath.Join("..", "internal", "embed", "onnxruntime.go"), `ONNXRuntimeVersion\s*=\s*"([^"]+)"`),
	}
	for name, got := range checks {
		if got != lock.ONNXRuntimeVersion {
			t.Fatalf("%s ONNX Runtime version = %q, want lock version %q", name, got, lock.ONNXRuntimeVersion)
		}
	}
}

func fixtureLock() runtimeLock {
	return runtimeLock{
		SchemaVersion:      2,
		ONNXRuntimeVersion: "1.25.1",
		NativeReleaseTag:   "thr-native-onnxruntime-v1.25.1",
		Targets: []runtimeTarget{
			{
				Target:               "darwin-arm64",
				Status:               "shipping",
				OS:                   "darwin",
				Arch:                 "arm64",
				Runner:               "macos-latest",
				Installer:            "unix",
				Source:               "official-release-asset",
				SourceURL:            "https://example.com/ort.tgz",
				SourceArchiveSHA256:  "source-sha",
				SourceLibraryPath:    "lib/libonnxruntime.dylib",
				RuntimeAssetName:     "thr-onnxruntime_1.25.1_darwin_arm64.tar.gz",
				RuntimeAssetURL:      "https://github.com/Chadi00/thr/releases/download/thr-native-onnxruntime-v1.25.1/thr-onnxruntime_1.25.1_darwin_arm64.tar.gz",
				RuntimeArchiveSHA256: "archive-sha",
				RuntimeLibraryPath:   "lib/libonnxruntime.dylib",
				RuntimeLibrarySHA256: "lib-sha",
				LicenseFiles:         []string{"LICENSE"},
			},
			{
				Target:               "darwin-amd64",
				Status:               "shipping",
				OS:                   "darwin",
				Arch:                 "amd64",
				Runner:               "macos-15-intel",
				Installer:            "unix",
				Source:               "source-build",
				SourceRepo:           "https://example.com/ort.git",
				SourceTag:            "v1.25.1",
				SourceLibraryPath:    "build/MacOS/Release/libonnxruntime.dylib",
				RuntimeAssetName:     "thr-onnxruntime_1.25.1_darwin_amd64.tar.gz",
				RuntimeAssetURL:      "https://github.com/Chadi00/thr/releases/download/thr-native-onnxruntime-v1.25.1/thr-onnxruntime_1.25.1_darwin_amd64.tar.gz",
				RuntimeArchiveSHA256: "archive-sha",
				RuntimeLibraryPath:   "lib/libonnxruntime.dylib",
				RuntimeLibrarySHA256: "lib-sha",
				LicenseFiles:         []string{"LICENSE"},
			},
			{
				Target:               "linux-amd64",
				Status:               "shipping",
				OS:                   "linux",
				Arch:                 "amd64",
				Runner:               "ubuntu-latest",
				Installer:            "unix",
				Source:               "official-release-asset",
				SourceURL:            "https://example.com/ort-linux.tgz",
				SourceArchiveSHA256:  "source-sha",
				SourceLibraryPath:    "lib/libonnxruntime.so",
				RuntimeAssetName:     "thr-onnxruntime_1.25.1_linux_amd64.tar.gz",
				RuntimeAssetURL:      "https://github.com/Chadi00/thr/releases/download/thr-native-onnxruntime-v1.25.1/thr-onnxruntime_1.25.1_linux_amd64.tar.gz",
				RuntimeArchiveSHA256: "archive-sha",
				RuntimeLibraryPath:   "lib/libonnxruntime.so",
				RuntimeLibrarySHA256: "lib-sha",
				LicenseFiles:         []string{"LICENSE"},
			},
			{
				Target:               "linux-arm64",
				Status:               "shipping",
				OS:                   "linux",
				Arch:                 "arm64",
				Runner:               "ubuntu-24.04-arm",
				Installer:            "unix",
				Source:               "official-release-asset",
				SourceURL:            "https://example.com/ort-linux-arm64.tgz",
				SourceArchiveSHA256:  "source-sha",
				SourceLibraryPath:    "lib/libonnxruntime.so",
				RuntimeAssetName:     "thr-onnxruntime_1.25.1_linux_arm64.tar.gz",
				RuntimeAssetURL:      "https://github.com/Chadi00/thr/releases/download/thr-native-onnxruntime-v1.25.1/thr-onnxruntime_1.25.1_linux_arm64.tar.gz",
				RuntimeArchiveSHA256: "archive-sha",
				RuntimeLibraryPath:   "lib/libonnxruntime.so",
				RuntimeLibrarySHA256: "lib-sha",
				LicenseFiles:         []string{"LICENSE"},
			},
		},
	}
}

func writeLock(t *testing.T, lock runtimeLock) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "onnxruntime.lock")
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func extractVersion(t *testing.T, path string, pattern string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(pattern).FindSubmatch(data)
	if len(matches) != 2 {
		t.Fatalf("could not find version in %s", path)
	}
	return string(matches[1])
}
