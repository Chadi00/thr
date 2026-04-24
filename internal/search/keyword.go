package search

import (
	"context"

	"github.com/Chadi00/thr/internal/domain"
	"github.com/Chadi00/thr/internal/store"
)

type KeywordSearcher struct {
	repo *store.Repository
}

func NewKeywordSearcher(repo *store.Repository) *KeywordSearcher {
	return &KeywordSearcher{repo: repo}
}

func (s *KeywordSearcher) Search(ctx context.Context, query string, limit int) ([]domain.Memory, error) {
	return s.repo.KeywordSearch(ctx, query, limit)
}
