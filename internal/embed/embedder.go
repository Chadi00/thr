package embed

// MaxMemoryTextCodePoints keeps passage text within the active model's token budget.
const MaxMemoryTextCodePoints = 508

type Embedder interface {
	PassageEmbed(text string) ([]float32, error)
	QueryEmbed(text string) ([]float32, error)
	Close() error
}
