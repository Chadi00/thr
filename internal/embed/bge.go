package embed

import (
	"fmt"
	"sync"

	fastembed "github.com/bdombro/fastembed-go"
)

type BGEEmbedder struct {
	model *fastembed.FlagEmbedding
	mu    sync.Mutex
}

func NewBGEEmbedder(cacheDir string) (*BGEEmbedder, error) {
	showProgress := false
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

func (e *BGEEmbedder) PassageEmbed(text string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

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
