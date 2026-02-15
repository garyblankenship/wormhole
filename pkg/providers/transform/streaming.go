package transform

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// StreamingConfig configures how to parse streaming responses for a provider
type StreamingConfig struct {
	// JSON path configurations for extracting fields
	TextFieldPath     string // e.g., "choices.0.delta.content", "candidates.0.content.parts.0.text"
	ToolCallFieldPath string // e.g., "choices.0.delta.tool_calls", "candidates.0.content.parts.0.functionCall"
	FinishReasonPath  string // e.g., "choices.0.finish_reason", "candidates.0.finishReason"
	UsagePath         string // e.g., "usage", "usageMetadata"
	IDPath            string // e.g., "id"
	ModelPath         string // e.g., "model"

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
				toolCalls, err := t.parseDefaultToolCalls(val)
				if err != nil {
					return nil, fmt.Errorf("failed to parse tool calls: %w", err)
				}
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

	// Extract finish reason
	if t.config.FinishReasonPath != "" {
		if val := t.getFieldByPath(response, t.config.FinishReasonPath); val != nil {
			var reasonStr string
			if str, ok := val.(string); ok {
				reasonStr = str
			} else if b, ok := val.(bool); ok {
				// Handle boolean finish reasons (e.g., Ollama's "done" field)
				if b {
					reasonStr = "true"
				} else {
					reasonStr = "false"
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

	// Extract usage
	if t.config.UsagePath != "" {
		if val := t.getFieldByPath(response, t.config.UsagePath); val != nil {
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

// ParseChunks parses multiple streaming chunks from JSON data (for batch-returning providers)
func (t *StreamingTransformer) ParseChunks(data []byte) ([]types.TextChunk, error) {
	if !t.config.ReturnsBatch {
		// For single-chunk providers, wrap ParseChunk result
		chunk, err := t.ParseChunk(data)
		if err != nil {
			return nil, err
		}
		if chunk != nil {
			return []types.TextChunk{*chunk}, nil
		}
		return nil, nil
	}

	// For batch providers, we need to handle multiple candidates/choices
	// This is provider-specific and requires custom logic
	// For now, return error - batch providers should use custom adapters
	return nil, fmt.Errorf("batch parsing requires provider-specific adapter")
}

// parseDefaultToolCalls parses tool calls from generic interface{}
func (t *StreamingTransformer) parseDefaultToolCalls(data any) ([]types.ToolCall, error) {
	var toolCalls []types.ToolCall

	switch v := data.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				tc, err := t.parseToolCallFromMap(m)
				if err != nil {
					return nil, err
				}
				if tc != nil {
					toolCalls = append(toolCalls, *tc)
				}
			}
		}
	case map[string]any:
		tc, err := t.parseToolCallFromMap(v)
		if err != nil {
			return nil, err
		}
		if tc != nil {
			toolCalls = append(toolCalls, *tc)
		}
	default:
		// Try to unmarshal JSON string
		if str, ok := data.(string); ok {
			var arr []map[string]any
			if err := json.Unmarshal([]byte(str), &arr); err == nil {
				for _, m := range arr {
					tc, err := t.parseToolCallFromMap(m)
					if err != nil {
						return nil, err
					}
					if tc != nil {
						toolCalls = append(toolCalls, *tc)
					}
				}
			}
		}
	}

	return toolCalls, nil
}

// parseToolCallFromMap parses a tool call from a map
func (t *StreamingTransformer) parseToolCallFromMap(m map[string]any) (*types.ToolCall, error) {
	tc := &types.ToolCall{}

	if id, ok := m["id"].(string); ok {
		tc.ID = id
	}
	if typ, ok := m["type"].(string); ok {
		tc.Type = typ
	}

	// Parse function call
	if functionMap, ok := m["function"].(map[string]any); ok {
		tc.Function = &types.ToolCallFunction{}
		if name, ok := functionMap["name"].(string); ok {
			tc.Function.Name = name
			tc.Name = name // Also set top-level Name field
		}
		if arguments, ok := functionMap["arguments"].(string); ok {
			tc.Function.Arguments = arguments
			// Try to parse arguments into map
			var argsMap map[string]any
			if err := json.Unmarshal([]byte(arguments), &argsMap); err == nil {
				tc.Arguments = argsMap
			}
		}
	}

	// Handle provider-specific formats
	if tc.Function == nil {
		if name, ok := m["name"].(string); ok {
			tc.Name = name
		}
		if args, ok := m["args"].(map[string]any); ok {
			tc.Arguments = args
			// Convert arguments to JSON string
			if argsBytes, err := json.Marshal(args); err == nil {
				tc.Function = &types.ToolCallFunction{
					Name:      tc.Name,
					Arguments: string(argsBytes),
				}
			}
		}
	}

	return tc, nil
}

// parseDefaultUsage parses usage information from generic interface{}
func (t *StreamingTransformer) parseDefaultUsage(data any) (*types.Usage, error) {
	usage := &types.Usage{}

	switch v := data.(type) {
	case map[string]any:
		if promptTokens, ok := v["prompt_tokens"].(float64); ok {
			usage.PromptTokens = int(promptTokens)
		}
		if completionTokens, ok := v["completion_tokens"].(float64); ok {
			usage.CompletionTokens = int(completionTokens)
		}
		if totalTokens, ok := v["total_tokens"].(float64); ok {
			usage.TotalTokens = int(totalTokens)
		}
		// Handle alternative field names
		if promptTokens, ok := v["promptTokenCount"].(float64); ok {
			usage.PromptTokens = int(promptTokens)
		}
		if completionTokens, ok := v["candidatesTokenCount"].(float64); ok {
			usage.CompletionTokens = int(completionTokens)
		}
		if totalTokens, ok := v["totalTokenCount"].(float64); ok {
			usage.TotalTokens = int(totalTokens)
		}
	case []byte:
		var m map[string]any
		if err := json.Unmarshal(v, &m); err != nil {
			return nil, err
		}
		return t.parseDefaultUsage(m)
	default:
		return nil, fmt.Errorf("unsupported usage data type: %T", data)
	}

	return usage, nil
}

// mapDefaultFinishReason maps a finish reason string to FinishReason enum
func (t *StreamingTransformer) mapDefaultFinishReason(reason string) types.FinishReason {
	reason = strings.ToLower(reason)
	switch reason {
	case "stop", "end_turn":
		return types.FinishReasonStop
	case "length", "max_tokens":
		return types.FinishReasonLength
	case "tool_calls", "function_call", "tool_use":
		return types.FinishReasonToolCalls
	case "content_filter":
		return types.FinishReasonContentFilter
	default:
		return types.FinishReasonStop
	}
}

// Predefined configurations for common providers

// NewOpenAIStreamingTransformer creates a transformer configured for OpenAI
func NewOpenAIStreamingTransformer() *StreamingTransformer {
	return NewStreamingTransformer(StreamingConfig{
		TextFieldPath:     "choices.0.delta.content",
		ToolCallFieldPath: "choices.0.delta.tool_calls",
		FinishReasonPath:  "choices.0.finish_reason",
		UsagePath:         "usage",
		IDPath:            "id",
		ModelPath:         "model",
		FinishReasonAdapter: func(reason string) types.FinishReason {
			switch reason {
			case "stop":
				return types.FinishReasonStop
			case "length":
				return types.FinishReasonLength
			case "tool_calls", "function_call":
				return types.FinishReasonToolCalls
			case "content_filter":
				return types.FinishReasonContentFilter
			default:
				return types.FinishReasonStop
			}
		},
		ReturnsBatch: false,
		ChunkType:    "text_chunk",
	})
}

// NewAnthropicStreamingTransformer creates a transformer configured for Anthropic
func NewAnthropicStreamingTransformer() *StreamingTransformer {
	return NewStreamingTransformer(StreamingConfig{
		// Anthropic uses event-based streaming, so paths depend on event type
		// This is a simplified configuration for basic text extraction
		TextFieldPath:    "delta.text",
		FinishReasonPath: "delta.stop_reason",
		UsagePath:        "usage",
		FinishReasonAdapter: func(reason string) types.FinishReason {
			switch reason {
			case "end_turn":
				return types.FinishReasonStop
			case "max_tokens":
				return types.FinishReasonLength
			case "tool_use":
				return types.FinishReasonToolCalls
			default:
				return types.FinishReasonStop
			}
		},
		ReturnsBatch: false,
		ChunkType:    "stream_chunk",
	})
}

// NewOllamaStreamingTransformer creates a transformer configured for Ollama
func NewOllamaStreamingTransformer() *StreamingTransformer {
	return NewStreamingTransformer(StreamingConfig{
		TextFieldPath:    "message.content",
		FinishReasonPath: "done", // Ollama uses boolean done field
		IDPath:           "",     // Ollama doesn't provide ID
		ModelPath:        "model",
		FinishReasonAdapter: func(reason string) types.FinishReason {
			// Ollama uses boolean done field, but we treat "true" as stop
			if reason == "true" {
				return types.FinishReasonStop
			}
			return types.FinishReasonStop
		},
		ReturnsBatch: false,
		ChunkType:    "text_chunk",
	})
}

// NewGeminiStreamingTransformer creates a transformer configured for Gemini
// Note: Gemini returns batches of chunks, so this requires custom handling
func NewGeminiStreamingTransformer() *StreamingTransformer {
	return NewStreamingTransformer(StreamingConfig{
		// Gemini requires custom adapter due to batch processing
		ReturnsBatch: true,
		ChunkType:    "text_chunk",
	})
}
