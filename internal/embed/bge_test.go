package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"testing"
)

func TestBGEEmbedderReturnsErrorAfterClose(t *testing.T) {
	embedder := &BGEEmbedder{}
	if err := embedder.Close(); err != nil {
		t.Fatalf("close zero-value embedder: %v", err)
	}
	if _, err := embedder.PassageEmbed("hello"); err == nil {
		t.Fatal("expected passage embed after close to fail")
	}
	if _, err := embedder.QueryEmbed("hello"); err == nil {
		t.Fatal("expected query embed after close to fail")
	}
}

func TestEnsureVerifiedActiveModelDownloadsAndReusesCache(t *testing.T) {
	files := map[string]string{
		"config.json":          `{"ok":true}`,
		"tokenizer.json":       `{"tokens":[]}`,
		"model_optimized.onnx": "onnx",
	}
	withTestManifest(t, files)

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		name := path.Base(r.URL.Path)
		value, ok := files[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(value))
	}))
	defer server.Close()
	activeModelBaseURL = server.URL

	cacheDir := t.TempDir()
	if err := EnsureVerifiedActiveModel(cacheDir, false); err != nil {
		t.Fatalf("ensure model: %v", err)
	}
	if got := requests; got != len(files) {
		t.Fatalf("expected %d downloads, got %d", len(files), got)
	}
	status := ActiveModelStatus(cacheDir)
	if !status.Verified {
		t.Fatalf("expected verified model status: %+v", status)
	}
	assertModeEmbed(t, filepath.Join(cacheDir, modelCacheName), 0o700)
	assertModeEmbed(t, filepath.Join(cacheDir, modelCacheName, "config.json"), 0o600)

	if err := EnsureVerifiedActiveModel(cacheDir, false); err != nil {
		t.Fatalf("ensure cached model: %v", err)
	}
	if got := requests; got != len(files) {
		t.Fatalf("expected cached model reuse without downloads, got %d requests", got)
	}
}

func TestEnsureVerifiedActiveModelRejectsDigestMismatch(t *testing.T) {
	files := map[string]string{"config.json": "expected"}
	withTestManifest(t, files)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tampered"))
	}))
	defer server.Close()
	activeModelBaseURL = server.URL

	cacheDir := t.TempDir()
	if err := EnsureVerifiedActiveModel(cacheDir, false); err == nil {
		t.Fatal("expected digest mismatch")
	}
	if _, err := os.Stat(filepath.Join(cacheDir, modelCacheName)); !os.IsNotExist(err) {
		t.Fatalf("expected failed download to clean cache dir, stat err=%v", err)
	}
}

func withTestManifest(t *testing.T, files map[string]string) {
	t.Helper()

	originalFiles := activeModelFiles
	originalBaseURL := activeModelBaseURL
	activeModelFiles = make([]modelFile, 0, len(files))
	for name, value := range files {
		sum := sha256.Sum256([]byte(value))
		activeModelFiles = append(activeModelFiles, modelFile{Name: name, SHA256: hex.EncodeToString(sum[:])})
	}
	t.Cleanup(func() {
		activeModelFiles = originalFiles
		activeModelBaseURL = originalBaseURL
	})
}

func assertModeEmbed(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s: got %o want %o", path, got, want)
	}
}
