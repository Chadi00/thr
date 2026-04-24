package embed

type Embedder interface {
	PassageEmbed(text string) ([]float32, error)
	QueryEmbed(text string) ([]float32, error)
	Close() error
}
