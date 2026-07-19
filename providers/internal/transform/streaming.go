package transform

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

// StreamingConfig configures how to parse streaming responses for a provider
type StreamingConfig struct {
	// JSON path configurations for extracting fields
	TextFieldPath         string // e.g., "choices.0.delta.content", "candidates.0.content.parts.0.text"
	ToolCallFieldPath     string // e.g., "choices.0.delta.tool_calls", "candidates.0.content.parts.0.functionCall"
	FinishReasonPath      string // e.g., "choices.0.finish_reason", "candidates.0.finishReason"
	UsagePath             string // e.g., "usage", "usageMetadata"
	IDPath                string // e.g., "id"
	ModelPath             string // e.g., "model"
	ThinkingPath          string // e.g., "choices.0.delta.reasoning_content"
	RefusalPath           string // e.g., "choices.0.delta.refusal"
	ExtraFinishReasonPath string // secondary path when FinishReasonPath is a bool true (e.g., Ollama "done_reason")

	// Field adapters for provider-specific formats
	TextAdapter         func(any) (string, error)
	ToolCallAdapter     func(any) (*types.ToolCall, error)
	UsageAdapter        func(any) (*types.Usage, error)
	FinishReasonAdapter func(string) types.FinishReason

	// Processing configuration
	ReturnsBatch bool   // true for providers that return multiple chunks per event (e.g., Gemini)
	ChunkType    string // "text_chunk" or "stream_chunk" (for backward compatibility)
}

// StreamingTransformer provides unified streaming response transformation
type StreamingTransformer struct {
	config StreamingConfig
}

// NewStreamingTransformer creates a new streaming transformer with the given configuration
func NewStreamingTransformer(config StreamingConfig) *StreamingTransformer {
	return &StreamingTransformer{
		config: config,
	}
}

// getFieldByPath extracts a field from a map using a dot-separated path
// Simple path support for: "field", "field.subfield", "array.0.field"
func (t *StreamingTransformer) getFieldByPath(data map[string]any, path string) any {
	if path == "" {
		return nil
	}

	parts := strings.Split(path, ".")
	current := any(data)

	for _, part := range parts {
		if current == nil {
			return nil
		}

		// Handle array index notation like "choices.0.delta.content"
		if idx, err := t.parseArrayIndex(part); err == nil {
			// part is an array index
			if arr, ok := current.([]any); ok && idx >= 0 && idx < len(arr) {
				current = arr[idx]
			} else {
				return nil
			}
		} else {
			// part is a map key
			if m, ok := current.(map[string]any); ok {
				current = m[part]
			} else {
				return nil
			}
		}
	}

	return current
}

// parseArrayIndex attempts to parse a string as an array index
func (t *StreamingTransformer) parseArrayIndex(s string) (int, error) {
	// Simple check: if string contains only digits
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number")
		}
	}
	// Convert string to int
	var result int
	for _, r := range s {
		result = result*10 + int(r-'0')
	}
	return result, nil
}

// ParseChunk parses a single streaming chunk from JSON data
func (t *StreamingTransformer) ParseChunk(data []byte) (*types.TextChunk, error) {
	if t.config.ReturnsBatch {
		// For batch-returning providers, use ParseChunks instead
		return nil, fmt.Errorf("provider returns batches, use ParseChunks instead")
	}

	// Parse JSON into map
	var response map[string]any
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// In-band provider error frame (common on OpenAI-compatible gateways like
	// OpenRouter/LiteLLM: `data: {"error":{...}}`). Surface it as a hard error so a
	// mid-stream failure isn't silently returned as an empty chunk / clean completion.
	if errVal, ok := response["error"]; ok && errVal != nil {
		if errObj, ok := errVal.(map[string]any); ok {
			msg, _ := errObj["message"].(string)
			typ, _ := errObj["type"].(string)
			if msg != "" || typ != "" {
				return nil, fmt.Errorf("provider stream error (%s): %s", typ, msg)
			}
		}
		return nil, fmt.Errorf("provider stream error: %s", string(data))
	}

	chunk := &types.TextChunk{}

	// Extract ID if path is configured
	if t.config.IDPath != "" {
		if val := t.getFieldByPath(response, t.config.IDPath); val != nil {
			if str, ok := val.(string); ok {
				chunk.ID = str
			}
		}
	}

	// Extract model if path is configured
	if t.config.ModelPath != "" {
		if val := t.getFieldByPath(response, t.config.ModelPath); val != nil {
			if str, ok := val.(string); ok {
				chunk.Model = str
			}
		}
	}

	// Extract text content
	if t.config.TextFieldPath != "" {
		if val := t.getFieldByPath(response, t.config.TextFieldPath); val != nil {
			if t.config.TextAdapter != nil {
				text, err := t.config.TextAdapter(val)
				if err != nil {
					return nil, fmt.Errorf("failed to adapt text: %w", err)
				}
				chunk.Text = text
			} else {
				// Default text extraction
				if str, ok := val.(string); ok {
					chunk.Text = str
				}
			}
		}
	}

	// Extract tool calls
	if t.config.ToolCallFieldPath != "" {
		if val := t.getFieldByPath(response, t.config.ToolCallFieldPath); val != nil {
			if t.config.ToolCallAdapter != nil {
				toolCall, err := t.config.ToolCallAdapter(val)
				if err != nil {
					return nil, fmt.Errorf("failed to adapt tool call: %w", err)
				}
				if toolCall != nil {
					chunk.ToolCall = toolCall
					// Also set plural field for consistency
					chunk.ToolCalls = []types.ToolCall{*toolCall}
				}
			} else {
				// Default tool call parsing
				toolCalls := t.parseDefaultToolCalls(val)
				if len(toolCalls) > 0 {
					// Always set plural ToolCalls field
					chunk.ToolCalls = toolCalls
					// Also set singular ToolCall for single tool call for compatibility
					if len(toolCalls) == 1 {
						chunk.ToolCall = &toolCalls[0]
					}
				}
			}
		}
	}

	// Extract thinking / reasoning content
	if t.config.ThinkingPath != "" {
		if val := t.getFieldByPath(response, t.config.ThinkingPath); val != nil {
			if str, ok := val.(string); ok && str != "" {
				thinking := &types.Thinking{Content: str}
				chunk.Thinking = thinking
				if chunk.Delta != nil {
					chunk.Delta.Thinking = thinking
				}
			}
		}
	}

	// Preserve refusal deltas separately from ordinary assistant text.
	if t.config.RefusalPath != "" {
		if val := t.getFieldByPath(response, t.config.RefusalPath); val != nil {
			if refusal, ok := val.(string); ok && refusal != "" {
				chunk.Refusal = refusal
				if chunk.Delta == nil {
					chunk.Delta = &types.ChunkDelta{}
				}
				chunk.Delta.Refusal = refusal
			}
		}
	}

	// Extract finish reason
	if t.config.FinishReasonPath != "" {
		if val := t.getFieldByPath(response, t.config.FinishReasonPath); val != nil {
			var reasonStr string
			if str, ok := val.(string); ok {
				reasonStr = str
			} else if b, ok := val.(bool); ok {
				// Boolean finish reason (e.g., Ollama's "done" field).
				// false = intermediate chunk, no finish reason. true = terminal.
				if b {
					if t.config.ExtraFinishReasonPath != "" {
						if extra := t.getFieldByPath(response, t.config.ExtraFinishReasonPath); extra != nil {
							if s, ok := extra.(string); ok && s != "" {
								reasonStr = s
							}
						}
					}
					if reasonStr == "" {
						reasonStr = "true"
					}
				}
			}

			if reasonStr != "" {
				if t.config.FinishReasonAdapter != nil {
					reason := t.config.FinishReasonAdapter(reasonStr)
					chunk.FinishReason = &reason
				} else {
					reason := t.mapDefaultFinishReason(reasonStr)
					chunk.FinishReason = &reason
				}
			}
		}
	}

	// Extract usage. An empty UsagePath means "use the root object" (e.g. Ollama).
	if t.config.UsagePath != "" || (t.config.UsagePath == "" && t.config.UsageAdapter != nil) {
		var val any
		if t.config.UsagePath == "" {
			val = response
		} else {
			val = t.getFieldByPath(response, t.config.UsagePath)
		}
		if val != nil {
			if t.config.UsageAdapter != nil {
				usage, err := t.config.UsageAdapter(val)
				if err != nil {
					return nil, fmt.Errorf("failed to adapt usage: %w", err)
				}
				chunk.Usage = usage
			} else {
				usage, err := t.parseDefaultUsage(val)
				if err != nil {
					return nil, fmt.Errorf("failed to parse usage: %w", err)
				}
				chunk.Usage = usage
			}
		}
	}

	// For OpenAI compatibility, set Delta field if Text is set but Delta is nil
	if chunk.Text != "" && chunk.Delta == nil {
		chunk.Delta = &types.ChunkDelta{
			Content: chunk.Text,
		}
	}

	return chunk, nil
}
