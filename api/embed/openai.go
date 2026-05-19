package embed

import (
	"context"
	"fmt"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/wikiSubmission/ws-backend/api/observability"
)

const embeddingModel = "text-embedding-3-small"

// OpenAIEmbedder calls text-embedding-3-small with 512 dimensions.
// Safe for concurrent use — the underlying HTTP client is shared.
type OpenAIEmbedder struct {
	client     *openai.Client
	dimensions int64
}

// NewOpenAIEmbedder returns an Embedder backed by OpenAI text-embedding-3-small at 512 dims.
func NewOpenAIEmbedder(apiKey string) *OpenAIEmbedder {
	c := openai.NewClient(option.WithAPIKey(apiKey))
	return &OpenAIEmbedder{
		client:     &c,
		dimensions: 512,
	}
}

// Embed encodes text into a 512-dimensional float32 vector.
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	start := time.Now()
	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input:          openai.EmbeddingNewParamsInputUnion{OfString: openai.String(text)},
		Model:          openai.EmbeddingModelTextEmbedding3Small,
		Dimensions:     openai.Int(e.dimensions),
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
	})
	observability.EmbeddingAPIDurationSeconds.WithLabelValues(embeddingModel).Observe(time.Since(start).Seconds())
	if err != nil {
		observability.EmbeddingRequestsTotal.WithLabelValues(embeddingModel, "error").Inc()
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	if len(resp.Data) == 0 {
		observability.EmbeddingRequestsTotal.WithLabelValues(embeddingModel, "empty").Inc()
		return nil, fmt.Errorf("openai embed: empty response")
	}
	observability.EmbeddingRequestsTotal.WithLabelValues(embeddingModel, "success").Inc()
	raw := resp.Data[0].Embedding
	out := make([]float32, len(raw))
	for i, v := range raw {
		out[i] = float32(v)
	}
	return out, nil
}
