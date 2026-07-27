package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
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

func TestMaxMemoryTextCodePointsFitsTokenizer(t *testing.T) {
	data, err := embeddedModelAssets.ReadFile("model_assets/tokenizer.json")
	if err != nil {
		t.Fatal(err)
	}
	tokenizerPath := filepath.Join(t.TempDir(), "tokenizer.json")
	if err := os.WriteFile(tokenizerPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	tknzer, err := pretrained.FromFile(tokenizerPath)
	if err != nil {
		t.Fatal(err)
	}
	tknzer.WithTruncation(&tokenizer.TruncationParams{MaxLength: 512, Strategy: tokenizer.LongestFirst})

	encode := func(text string) tokenizer.Encoding {
		t.Helper()
		inputs := []tokenizer.EncodeInput{tokenizer.NewSingleEncodeInput(tokenizer.NewInputSequence("passage: " + text))}
		encodings, err := tknzer.EncodeBatch(inputs, true)
		if err != nil {
			t.Fatal(err)
		}
		return encodings[0]
	}

	atLimit := encode(strings.Repeat("!", MaxMemoryTextCodePoints))
	if atLimit.Len() != 512 || len(atLimit.GetOverflowing()) != 0 {
		t.Fatalf("expected limit to fit exactly, got length %d and %d overflows", atLimit.Len(), len(atLimit.GetOverflowing()))
	}
	overLimit := encode(strings.Repeat("!", MaxMemoryTextCodePoints+1))
	if len(overLimit.GetOverflowing()) == 0 {
		t.Fatal("expected one additional code point to overflow the tokenizer")
	}
}

func TestEnsureVerifiedActiveModelPreparesAndReusesCache(t *testing.T) {
	modelFiles := map[string]string{
		"config.json":          `{"ok":true}`,
		"tokenizer.json":       `{"tokens":[]}`,
		"model_optimized.onnx": "onnx",
	}
	assetFiles := map[string]string{
		"config.json":                   `{"ok":true}`,
		"tokenizer.json":                `{"tokens":[]}`,
		"model_optimized.onnx.part-001": "nx",
		"model_optimized.onnx.part-000": "on",
	}
	withTestModelAssets(t, modelFiles, assetFiles)

	cacheDir := t.TempDir()
	if err := EnsureVerifiedActiveModel(cacheDir, false); err != nil {
		t.Fatalf("ensure model: %v", err)
	}
	status := ActiveModelStatus(cacheDir)
	if !status.Verified {
		t.Fatalf("expected verified model status: %+v", status)
	}
	assertModeEmbed(t, filepath.Join(cacheDir, modelCacheName), 0o700)
	assertModeEmbed(t, filepath.Join(cacheDir, modelCacheName, "config.json"), 0o600)
	gotONNX, err := os.ReadFile(filepath.Join(cacheDir, modelCacheName, "model_optimized.onnx"))
	if err != nil {
		t.Fatalf("read assembled onnx: %v", err)
	}
	if string(gotONNX) != "onnx" {
		t.Fatalf("unexpected assembled onnx content: %q", string(gotONNX))
	}

	activeModelAssets = modelAssetSource{filesystem: failingFS{}}
	if err := EnsureVerifiedActiveModel(cacheDir, false); err != nil {
		t.Fatalf("ensure cached model: %v", err)
	}
}

func TestEnsureVerifiedActiveModelRejectsDigestMismatch(t *testing.T) {
	modelFiles := map[string]string{"config.json": "expected"}
	assetFiles := map[string]string{"config.json": "tampered"}
	withTestModelAssets(t, modelFiles, assetFiles)

	cacheDir := t.TempDir()
	if err := EnsureVerifiedActiveModel(cacheDir, false); err == nil {
		t.Fatal("expected digest mismatch")
	}
	if _, err := os.Stat(filepath.Join(cacheDir, modelCacheName)); !os.IsNotExist(err) {
		t.Fatalf("expected failed prepare to clean cache dir, stat err=%v", err)
	}
}

func TestBundledActiveModelAssetsMatchManifest(t *testing.T) {
	dir := t.TempDir()
	for _, file := range activeModelFiles {
		if err := writeBundledModelFile(activeModelAssets, file, filepath.Join(dir, file.Name)); err != nil {
			t.Fatalf("write bundled model file %s: %v", file.Name, err)
		}
	}
}

func withTestModelAssets(t *testing.T, modelFiles map[string]string, assetFiles map[string]string) {
	t.Helper()

	originalFiles := activeModelFiles
	originalAssets := activeModelAssets
	activeModelFiles = make([]modelFile, 0, len(modelFiles))
	names := make([]string, 0, len(modelFiles))
	for name := range modelFiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := modelFiles[name]
		sum := sha256.Sum256([]byte(value))
		activeModelFiles = append(activeModelFiles, modelFile{Name: name, SHA256: hex.EncodeToString(sum[:])})
	}
	assetMap := fstest.MapFS{}
	for name, value := range assetFiles {
		assetMap[name] = &fstest.MapFile{Data: []byte(value), Mode: 0o444}
	}
	activeModelAssets = modelAssetSource{filesystem: assetMap}
	t.Cleanup(func() {
		activeModelFiles = originalFiles
		activeModelAssets = originalAssets
	})
}

type failingFS struct{}

func (failingFS) Open(name string) (fs.File, error) {
	return nil, errors.New("unexpected embedded model asset access")
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
