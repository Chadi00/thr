package search

import (
	"context"
	"fmt"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/embed"
	"github.com/Chadi00/thr/internal/store"
)

type SemanticResult struct {
	Memory   domain.Memory
	Distance float64
}

type SemanticSearcher struct {
	repo     *store.Repository
	embedder embed.Embedder
}

func NewSemanticSearcher(repo *store.Repository, embedder embed.Embedder) *SemanticSearcher {
	return &SemanticSearcher{repo: repo, embedder: embedder}
}

func (s *SemanticSearcher) Ask(ctx context.Context, question string, limit int) ([]SemanticResult, error) {
	vector, err := s.embedder.QueryEmbed(question)
	if err != nil {
		return nil, fmt.Errorf("create semantic query embedding: %w", err)
	}

	hits, err := s.repo.SemanticSearch(ctx, vector, limit)
	if err != nil {
		return nil, err
	}

	results := make([]SemanticResult, 0, len(hits))
	for _, hit := range hits {
		results = append(results, SemanticResult{
			Memory:   hit.Memory,
			Distance: hit.Distance,
		})
	}

	return results, nil
}
