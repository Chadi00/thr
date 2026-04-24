package embed

import "testing"

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
