package embed

import "context"

// Embedder generates a dense vector for a text string.
// Implementations must be safe for concurrent use.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
