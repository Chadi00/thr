package embed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/Chadi00/thr/internal/privacy"
	fastembed "github.com/bdombro/fastembed-go"
)

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

var activeModelBaseURL = "https://huggingface.co"

type BGEEmbedder struct {
	model *fastembed.FlagEmbedding
	mu    sync.Mutex
}

func NewBGEEmbedder(cacheDir string, showDownloadProgress bool) (*BGEEmbedder, error) {
	if err := EnsureVerifiedActiveModel(cacheDir, showDownloadProgress); err != nil {
		return nil, err
	}

	showProgress := showDownloadProgress
	options := fastembed.InitOptions{
		Model:                fastembed.BGEBaseENV15,
		CacheDir:             cacheDir,
		MaxLength:            512,
		ShowDownloadProgress: &showProgress,
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

func EnsureVerifiedActiveModel(cacheDir string, showDownloadProgress bool) error {
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
	if err := downloadActiveModel(cacheDir, showDownloadProgress); err != nil {
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

func downloadActiveModel(cacheDir string, showDownloadProgress bool) error {
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
		if showDownloadProgress {
			fmt.Fprintf(os.Stderr, "Downloading %s...\n", file.Name)
		}
		if err := downloadAndVerify(file, filepath.Join(partDir, file.Name)); err != nil {
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

func downloadAndVerify(file modelFile, path string) error {
	url := fmt.Sprintf("%s/%s/resolve/%s/%s", activeModelBaseURL, ActiveModelID, ActiveModelRevision, file.Name)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "thr (https://github.com/Chadi00/thr)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", file.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("download %s: %s", file.Name, resp.Status)
	}

	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, privacy.PrivateFileMode)
	if err != nil {
		return fmt.Errorf("create model file %s: %w", file.Name, err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, hash), resp.Body)
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
