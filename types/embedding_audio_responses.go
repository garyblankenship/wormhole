package types

import (
	"time"
)

// EmbeddingsResponse represents an embeddings response
type EmbeddingsResponse struct {
	ID         string         `json:"id"`
	Provider   string         `json:"provider,omitempty"`
	Model      string         `json:"model"`
	Embeddings []Embedding    `json:"embeddings"`
	Usage      *Usage         `json:"usage,omitempty"`
	Created    time.Time      `json:"created"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// RerankResult is a single reranked document with its relevance score.
type RerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
	Document       string  `json:"document"`
}

// RerankResponse represents a rerank response; Results are sorted by relevance descending.
type RerankResponse struct {
	ID       string         `json:"id"`
	Provider string         `json:"provider,omitempty"`
	Model    string         `json:"model"`
	Results  []RerankResult `json:"results"`
	Usage    *Usage         `json:"usage,omitempty"`
	Created  time.Time      `json:"created"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Content returns the first embedding vector, or nil if empty.
// For multiple embeddings, use Embeddings directly.
func (r *EmbeddingsResponse) Content() []float64 {
	if len(r.Embeddings) == 0 {
		return nil
	}
	return r.Embeddings[0].Embedding
}

// Vector returns the embedding vector at the given index.
// Returns nil if index is out of bounds.
func (r *EmbeddingsResponse) Vector(index int) []float64 {
	if index < 0 || index >= len(r.Embeddings) {
		return nil
	}
	return r.Embeddings[index].Embedding
}

// Count returns the number of embeddings in the response.
func (r *EmbeddingsResponse) Count() int {
	return len(r.Embeddings)
}

// Embedding represents a single embedding vector
type Embedding struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
	Base64    string    `json:"base64,omitempty"`
}

// ImagesResponse represents an image generation response
type ImagesResponse struct {
	ID       string           `json:"id"`
	Model    string           `json:"model"`
	Images   []GeneratedImage `json:"images"`
	Created  time.Time        `json:"created"`
	Metadata map[string]any   `json:"metadata,omitempty"`
}

// GeneratedImage represents a generated image
type GeneratedImage struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}

// ImageResponse represents an image generation response (alias for ImagesResponse)
type ImageResponse = ImagesResponse

// SpeechToTextResponse represents a speech-to-text response
type SpeechToTextResponse struct {
	ID       string         `json:"id,omitempty"`
	Model    string         `json:"model,omitempty"`
	Text     string         `json:"text"`
	Language string         `json:"language,omitempty"`
	Duration *float64       `json:"duration,omitempty"`
	Created  time.Time      `json:"created,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TextToSpeechResponse represents a text-to-speech response
type TextToSpeechResponse struct {
	ID       string         `json:"id,omitempty"`
	Model    string         `json:"model,omitempty"`
	Audio    []byte         `json:"audio"`
	Format   string         `json:"format,omitempty"`
	Created  time.Time      `json:"created,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// AudioResponse represents an audio response
type AudioResponse struct {
	ID       string         `json:"id,omitempty"`
	Model    string         `json:"model,omitempty"`
	Audio    []byte         `json:"audio,omitempty"` // For TTS
	Text     string         `json:"text,omitempty"`  // For STT
	Format   string         `json:"format,omitempty"`
	Created  time.Time      `json:"created,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
