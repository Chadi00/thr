package embed

import (
	"crypto/sha256"
	stdembed "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Chadi00/thr/internal/privacy"
	fastembed "github.com/bdombro/fastembed-go"
)

//go:embed model_assets/*
var embeddedModelAssets stdembed.FS

const (
	ActiveModelID        = "Qdrant/bge-base-en-v1.5-onnx-Q"
	ActiveModelRevision  = "738cad1c108e2f23649db9e44b2eab988626493b"
	ActiveModelDimension = 768
	modelCacheName       = "fast-bge-base-en-v1.5"
	verifiedManifest     = "thr_model_manifest.json"
)

type ModelIdentity struct {
	ModelID        string `json:"model_id"`
	ModelRevision  string `json:"model_revision"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Dimension      int    `json:"dimension"`
}

type ModelStatus struct {
	ModelIdentity
	Verified bool `json:"verified"`
}

type modelFile struct {
	Name   string
	SHA256 string
}

var activeModelFiles = []modelFile{
	{Name: "config.json", SHA256: "86f84a5285de7f1ee673f712387219ef1e261ec27dcd870e793a80f9da1aaa3b"},
	{Name: "model_optimized.onnx", SHA256: "4e556722bc4f65716c544c8a931f1e90fb3f866e5741fd93a96f051d673339c7"},
	{Name: "special_tokens_map.json", SHA256: "5d5b662e421ea9fac075174bb0688ee0d9431699900b90662acd44b2a350503a"},
	{Name: "tokenizer.json", SHA256: "d241a60d5e8f04cc1b2b3e9ef7a4921b27bf526d9f6050ab90f9267a1f9e5c66"},
	{Name: "tokenizer_config.json", SHA256: "0b29c7bfc889e53b36d9dd3e686dd4300f6525110eaa98c76a5dafceb2029f53"},
	{Name: "vocab.txt", SHA256: "07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3"},
}

type modelAssetSource struct {
	filesystem fs.FS
	root       string
}

var activeModelAssets = modelAssetSource{filesystem: embeddedModelAssets, root: "model_assets"}

type BGEEmbedder struct {
	model *fastembed.FlagEmbedding
	mu    sync.Mutex
}

func NewBGEEmbedder(cacheDir string, showPrepareProgress bool) (*BGEEmbedder, error) {
	if err := EnsureVerifiedActiveModel(cacheDir, showPrepareProgress); err != nil {
		return nil, err
	}
	onnxLibraryPath, err := resolveONNXRuntimeLibrary()
	if err != nil {
		return nil, err
	}

	showProgress := showPrepareProgress
	options := fastembed.InitOptions{
		Model:                  fastembed.BGEBaseENV15,
		ONNXRuntimeLibraryPath: onnxLibraryPath,
		CacheDir:               cacheDir,
		MaxLength:              512,
		ShowDownloadProgress:   &showProgress,
	}

	model, err := fastembed.NewFlagEmbedding(&options)
	if err != nil {
		return nil, fmt.Errorf("initialize bge-base-en-v1.5 model (check ONNX Runtime installation/path): %w", err)
	}

	return &BGEEmbedder{model: model}, nil
}

func ActiveModelIdentityValue() ModelIdentity {
	return ModelIdentity{
		ModelID:        ActiveModelID,
		ModelRevision:  ActiveModelRevision,
		ManifestSHA256: activeManifestSHA256(),
		Dimension:      ActiveModelDimension,
	}
}

func ActiveModelStatus(cacheDir string) ModelStatus {
	identity := ActiveModelIdentityValue()
	return ModelStatus{
		ModelIdentity: identity,
		Verified:      verifyModelDir(activeModelDir(cacheDir)) == nil,
	}
}

func EnsureVerifiedActiveModel(cacheDir string, showPrepareProgress bool) error {
	if err := privacy.EnsurePrivateDir(cacheDir); err != nil {
		return err
	}

	destDir := activeModelDir(cacheDir)
	if err := verifyModelDir(destDir); err == nil {
		return privacy.HardenTreeIfExists(cacheDir)
	}

	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("remove unverified model cache: %w", err)
	}
	if err := prepareActiveModel(cacheDir, showPrepareProgress); err != nil {
		return err
	}
	return privacy.HardenTreeIfExists(cacheDir)
}

func activeModelDir(cacheDir string) string {
	return filepath.Join(cacheDir, modelCacheName)
}

func activeManifestSHA256() string {
	files := append([]modelFile(nil), activeModelFiles...)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	hash := sha256.New()
	fmt.Fprintf(hash, "model_id=%s\nrevision=%s\ndimension=%d\n", ActiveModelID, ActiveModelRevision, ActiveModelDimension)
	for _, file := range files {
		fmt.Fprintf(hash, "%s=%s\n", file.Name, file.SHA256)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func verifyModelDir(dir string) error {
	for _, file := range activeModelFiles {
		got, err := fileSHA256(filepath.Join(dir, file.Name))
		if err != nil {
			return err
		}
		if got != file.SHA256 {
			return fmt.Errorf("model file %s digest mismatch", file.Name)
		}
	}

	manifest, err := os.ReadFile(filepath.Join(dir, verifiedManifest))
	if err != nil {
		return err
	}
	var identity ModelIdentity
	if err := json.Unmarshal(manifest, &identity); err != nil {
		return err
	}
	if identity != ActiveModelIdentityValue() {
		return errors.New("model manifest identity mismatch")
	}
	return nil
}

func prepareActiveModel(cacheDir string, showPrepareProgress bool) error {
	destDir := activeModelDir(cacheDir)
	partDir := destDir + ".partial"
	_ = os.RemoveAll(partDir)
	if err := privacy.EnsurePrivateDir(partDir); err != nil {
		return err
	}

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(partDir)
		}
	}()

	for _, file := range activeModelFiles {
		if showPrepareProgress {
			fmt.Fprintf(os.Stderr, "Preparing %s...\n", file.Name)
		}
		if err := writeBundledModelFile(activeModelAssets, file, filepath.Join(partDir, file.Name)); err != nil {
			return err
		}
	}

	manifest, err := json.MarshalIndent(ActiveModelIdentityValue(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode model manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(partDir, verifiedManifest), append(manifest, '\n'), privacy.PrivateFileMode); err != nil {
		return fmt.Errorf("write model manifest: %w", err)
	}
	if err := os.Rename(partDir, destDir); err != nil {
		return fmt.Errorf("publish verified model cache: %w", err)
	}
	cleanup = false
	return nil
}

func writeBundledModelFile(source modelAssetSource, file modelFile, path string) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, privacy.PrivateFileMode)
	if err != nil {
		return fmt.Errorf("create model file %s: %w", file.Name, err)
	}
	hash := sha256.New()
	copyErr := copyBundledModelFile(source, file.Name, io.MultiWriter(out, hash))
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("write model file %s: %w", file.Name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close model file %s: %w", file.Name, closeErr)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != file.SHA256 {
		return fmt.Errorf("verify model file %s: digest mismatch", file.Name)
	}
	return nil
}

func copyBundledModelFile(source modelAssetSource, name string, writer io.Writer) error {
	if name == "model_optimized.onnx" {
		chunks, err := source.chunkNames(name)
		if err != nil {
			return err
		}
		for _, chunk := range chunks {
			if err := copyBundledAsset(source, chunk, writer); err != nil {
				return err
			}
		}
		return nil
	}
	return copyBundledAsset(source, name, writer)
}

func copyBundledAsset(source modelAssetSource, name string, writer io.Writer) error {
	file, err := source.open(name)
	if err != nil {
		return fmt.Errorf("open embedded model asset %s: %w", name, err)
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	return err
}

func (s modelAssetSource) open(name string) (fs.File, error) {
	return s.filesystem.Open(s.assetPath(name))
}

func (s modelAssetSource) chunkNames(name string) ([]string, error) {
	entries, err := fs.ReadDir(s.filesystem, s.dir())
	if err != nil {
		return nil, fmt.Errorf("read embedded model assets: %w", err)
	}

	prefix := name + ".part-"
	chunks := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		chunks = append(chunks, entry.Name())
	}
	sort.Strings(chunks)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no embedded chunks found for %s", name)
	}
	return chunks, nil
}

func (s modelAssetSource) assetPath(name string) string {
	if s.root == "" {
		return name
	}
	return path.Join(s.root, name)
}

func (s modelAssetSource) dir() string {
	if s.root == "" {
		return "."
	}
	return s.root
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (e *BGEEmbedder) PassageEmbed(text string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.model == nil {
		return nil, errors.New("embedder is closed")
	}

	vectors, err := e.model.PassageEmbed([]string{text}, 1)
	if err != nil {
		return nil, fmt.Errorf("embed passage text: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("unexpected passage embedding count: %d", len(vectors))
	}
	return vectors[0], nil
}

func (e *BGEEmbedder) QueryEmbed(text string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.model == nil {
		return nil, errors.New("embedder is closed")
	}

	vector, err := e.model.QueryEmbed(text)
	if err != nil {
		return nil, fmt.Errorf("embed query text: %w", err)
	}
	return vector, nil
}

func (e *BGEEmbedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.model != nil {
		e.model.Destroy()
		e.model = nil
	}
	return nil
}
