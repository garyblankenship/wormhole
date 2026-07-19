package types

import (
	"encoding/json"
	"time"

	"github.com/garyblankenship/wormhole/v2/internal/pool"
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
	// CacheReadTokens is the number of prompt tokens served from a provider cache
	// (OpenAI cached_tokens, Anthropic cache_read_input_tokens). Cache reads
	// typically bill at a fraction of normal prompt-token cost.
	CacheReadTokens int `json:"cache_read_tokens,omitempty"`
	// CacheWriteTokens is the number of prompt tokens written into a provider
	// cache (Anthropic cache_creation_input_tokens). Zero for providers that do
	// not report cache writes.
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
	// ReasoningTokens is the number of completion tokens spent on chain-of-thought
	// reasoning (OpenAI o-series reasoning_tokens). Zero for providers/models
	// that do not report reasoning.
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// IsZero reports whether the Usage carries no token counts. Used to avoid
// clobbering a populated usage with an empty "usage":{} payload that some
// OpenAI-compatible proxies emit on a trailing stream chunk.
func (u Usage) IsZero() bool {
	return u.PromptTokens == 0 && u.CompletionTokens == 0 && u.TotalTokens == 0 &&
		u.CacheReadTokens == 0 && u.CacheWriteTokens == 0 && u.ReasoningTokens == 0
}

// TextResponse represents a text generation response
type TextResponse struct {
	ID           string         `json:"id"`
	Provider     string         `json:"provider,omitempty"`
	Model        string         `json:"model"`
	Text         string         `json:"text"`
	Refusal      string         `json:"refusal,omitempty"`
	Thinking     *Thinking      `json:"thinking,omitempty"`
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

	// Marshal and unmarshal for type conversion using pooled buffer
	jsonBytes, err := pool.Marshal(r.Data)
	if err != nil {
		return err
	}
	defer pool.Return(jsonBytes)
	return json.Unmarshal(jsonBytes, target)
}

// StreamChunk represents a streaming response chunk (alias for TextChunk)
type StreamChunk = TextChunk

// TextChunk represents a streaming text response chunk
type TextChunk struct {
	ID           string        `json:"id,omitempty"`
	Provider     string        `json:"provider,omitempty"`
	Model        string        `json:"model,omitempty"`
	Text         string        `json:"text,omitempty"`
	Refusal      string        `json:"refusal,omitempty"`
	Thinking     *Thinking     `json:"thinking,omitempty"`
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
	Refusal   string     `json:"refusal,omitempty"`
	Thinking  *Thinking  `json:"thinking,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Thinking carries provider-reported reasoning text or a provider signature
// associated with reasoning content.
type Thinking struct {
	Content   string `json:"content,omitempty"`
	Signature string `json:"signature,omitempty"`
	// Provider records which provider produced this thinking block, so a foreign signature is never replayed
	// to a provider that cannot accept it.
	Provider string `json:"provider,omitempty"`
}
