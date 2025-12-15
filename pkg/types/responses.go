package types

import (
	"encoding/json"
	"time"
)

// FinishReason represents why the model stopped generating
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonOther         FinishReason = "other"
)

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// TextResponse represents a text generation response
type TextResponse struct {
	ID           string         `json:"id"`
	Model        string         `json:"model"`
	Text         string         `json:"text"`
	ToolCalls    []ToolCall     `json:"tool_calls,omitempty"`
	FinishReason FinishReason   `json:"finish_reason"`
	Usage        *Usage         `json:"usage,omitempty"`
	Created      time.Time      `json:"created"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Content returns the text content of the response.
// This provides a unified accessor pattern across all response types.
func (r *TextResponse) Content() string {
	return r.Text
}

// HasToolCalls returns true if the response contains tool calls.
// Use this to check if the model wants to invoke tools before accessing ToolCalls.
func (r *TextResponse) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// IsComplete returns true if generation finished normally (not truncated).
func (r *TextResponse) IsComplete() bool {
	return r.FinishReason == FinishReasonStop
}

// WasTruncated returns true if the response was cut off due to length limits.
func (r *TextResponse) WasTruncated() bool {
	return r.FinishReason == FinishReasonLength
}

// StructuredResponse represents a structured output response
type StructuredResponse struct {
	ID       string         `json:"id"`
	Model    string         `json:"model"`
	Data     any            `json:"data"`
	Raw      string         `json:"raw,omitempty"`
	Usage    *Usage         `json:"usage,omitempty"`
	Created  time.Time      `json:"created"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Content returns the parsed data from the response.
// This provides a unified accessor pattern across all response types.
// Use type assertion or json.Unmarshal for type-safe access.
func (r *StructuredResponse) Content() any {
	return r.Data
}

// ContentAs unmarshals the response data into the provided target.
// This is a convenience method equivalent to json.Unmarshal(json.Marshal(r.Data), target).
//
// Example:
//
//	var person struct { Name string `json:"name"` }
//	if err := resp.ContentAs(&person); err != nil {
//	    log.Fatal(err)
//	}
func (r *StructuredResponse) ContentAs(target any) error {
	// Fast path: if Data is already the target type
	if r.Data == nil {
		return nil
	}

	// Marshal and unmarshal for type conversion
	jsonBytes, err := json.Marshal(r.Data)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonBytes, target)
}

// StreamChunk represents a streaming response chunk (alias for TextChunk)
type StreamChunk = TextChunk

// TextChunk represents a streaming text response chunk
type TextChunk struct {
	ID           string        `json:"id,omitempty"`
	Model        string        `json:"model,omitempty"`
	Text         string        `json:"text,omitempty"`
	Delta        *ChunkDelta   `json:"delta,omitempty"` // For OpenAI compatibility
	ToolCall     *ToolCall     `json:"tool_call,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"` // For multi-tool calls
	FinishReason *FinishReason `json:"finish_reason,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
	Error        error         `json:"-"`
}

// Content returns the text content of the chunk.
// Handles both direct Text field and Delta.Content for provider compatibility.
func (c *TextChunk) Content() string {
	if c.Text != "" {
		return c.Text
	}
	if c.Delta != nil {
		return c.Delta.Content
	}
	return ""
}

// IsDone returns true if this is the final chunk in the stream.
func (c *TextChunk) IsDone() bool {
	return c.FinishReason != nil && *c.FinishReason != ""
}

// HasError returns true if the chunk contains an error.
func (c *TextChunk) HasError() bool {
	return c.Error != nil
}

// HasToolCalls returns true if the chunk contains tool calls.
func (c *TextChunk) HasToolCalls() bool {
	return c.ToolCall != nil || len(c.ToolCalls) > 0
}

// ChunkDelta represents streaming delta content
type ChunkDelta struct {
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// EmbeddingsResponse represents an embeddings response
type EmbeddingsResponse struct {
	ID         string         `json:"id"`
	Model      string         `json:"model"`
	Embeddings []Embedding    `json:"embeddings"`
	Usage      *Usage         `json:"usage,omitempty"`
	Created    time.Time      `json:"created"`
	Metadata   map[string]any `json:"metadata,omitempty"`
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

// Error types
type WormholeProviderError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param,omitempty"`
}

func (e WormholeProviderError) Error() string {
	return e.Message
}

// OCRResponse represents an OCR (Optical Character Recognition) response
type OCRResponse struct {
	ID       string         `json:"id"`
	Model    string         `json:"model"`
	Text     string         `json:"text"`
	Created  int64          `json:"created"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
