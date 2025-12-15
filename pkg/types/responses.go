package types

import (
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
