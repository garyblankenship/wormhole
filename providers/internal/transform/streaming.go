package transform

import (
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/v3/types"
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
	paths  streamingPaths
}

// NewStreamingTransformer creates a new streaming transformer with the given configuration
func NewStreamingTransformer(config StreamingConfig) *StreamingTransformer {
	return &StreamingTransformer{
		config: config,
		paths:  compileStreamingPaths(config),
	}
}

// ParseChunk parses a single streaming chunk from JSON data
func (t *StreamingTransformer) ParseChunk(data []byte) (*types.TextChunk, error) {
	chunk, _, err := t.ParseChunkWithMap(data)
	return chunk, err
}

// ParseChunkWithMap parses a single streaming chunk and also returns the
// decoded root JSON object. Callers can extract provider-specific fields (for
// example a raw usage object for fidelity metadata) from the same decode
// instead of unmarshaling the chunk a second time. The root map is fresh per
// call; callers that embed sub-maps into the returned chunk (as provider_usage
// metadata does) intentionally retain that subtree, so no defensive copy is
// needed.
func (t *StreamingTransformer) ParseChunkWithMap(data []byte) (*types.TextChunk, map[string]any, error) {
	if t.config.ReturnsBatch {
		// For batch-returning providers, use ParseChunks instead
		return nil, nil, fmt.Errorf("provider returns batches, use ParseChunks instead")
	}

	// Parse JSON into map
	var response map[string]any
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// In-band provider error frame (common on OpenAI-compatible gateways like
	// OpenRouter/LiteLLM: `data: {"error":{...}}`). Surface it as a hard error so a
	// mid-stream failure isn't silently returned as an empty chunk / clean completion.
	if errVal, ok := response["error"]; ok && errVal != nil {
		if errObj, ok := errVal.(map[string]any); ok {
			msg, _ := errObj["message"].(string)
			typ, _ := errObj["type"].(string)
			if msg != "" || typ != "" {
				return nil, response, fmt.Errorf("provider stream error (%s): %s", typ, msg)
			}
		}
		return nil, response, fmt.Errorf("provider stream error: %s", string(data))
	}

	chunk := &types.TextChunk{}

	// Extract ID if path is configured
	if len(t.paths.id) > 0 {
		if val := getField(response, t.paths.id); val != nil {
			if str, ok := val.(string); ok {
				chunk.ID = str
			}
		}
	}

	// Extract model if path is configured
	if len(t.paths.model) > 0 {
		if val := getField(response, t.paths.model); val != nil {
			if str, ok := val.(string); ok {
				chunk.Model = str
			}
		}
	}

	// Extract text content
	if len(t.paths.textField) > 0 {
		if val := getField(response, t.paths.textField); val != nil {
			if t.config.TextAdapter != nil {
				text, err := t.config.TextAdapter(val)
				if err != nil {
					return nil, response, fmt.Errorf("failed to adapt text: %w", err)
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
	if len(t.paths.toolCallField) > 0 {
		if val := getField(response, t.paths.toolCallField); val != nil {
			if t.config.ToolCallAdapter != nil {
				toolCall, err := t.config.ToolCallAdapter(val)
				if err != nil {
					return nil, response, fmt.Errorf("failed to adapt tool call: %w", err)
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
	if len(t.paths.thinking) > 0 {
		if val := getField(response, t.paths.thinking); val != nil {
			if str, ok := val.(string); ok && str != "" {
				thinking := &types.Thinking{Content: str}
				chunk.Thinking = thinking
				if chunk.Delta == nil {
					chunk.Delta = &types.ChunkDelta{}
				}
				chunk.Delta.Thinking = thinking
			}
		}
	}

	// Preserve refusal deltas separately from ordinary assistant text.
	if len(t.paths.refusal) > 0 {
		if val := getField(response, t.paths.refusal); val != nil {
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
	if len(t.paths.finishReason) > 0 {
		if val := getField(response, t.paths.finishReason); val != nil {
			var reasonStr string
			if str, ok := val.(string); ok {
				reasonStr = str
			} else if b, ok := val.(bool); ok {
				// Boolean finish reason (e.g., Ollama's "done" field).
				// false = intermediate chunk, no finish reason. true = terminal.
				if b {
					if len(t.paths.extraFinishReason) > 0 {
						if extra := getField(response, t.paths.extraFinishReason); extra != nil {
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
	if len(t.paths.usage) > 0 || (len(t.paths.usage) == 0 && t.config.UsageAdapter != nil) {
		var val any
		if len(t.paths.usage) == 0 {
			val = response
		} else {
			val = getField(response, t.paths.usage)
		}
		if val != nil {
			if t.config.UsageAdapter != nil {
				usage, err := t.config.UsageAdapter(val)
				if err != nil {
					return nil, response, fmt.Errorf("failed to adapt usage: %w", err)
				}
				chunk.Usage = usage
			} else {
				usage, err := t.parseDefaultUsage(val)
				if err != nil {
					return nil, response, fmt.Errorf("failed to parse usage: %w", err)
				}
				chunk.Usage = usage
			}
		}
	}

	// Merge text with reasoning and refusal already present in the delta.
	if chunk.Text != "" {
		if chunk.Delta == nil {
			chunk.Delta = &types.ChunkDelta{}
		}
		chunk.Delta.Content = chunk.Text
	}

	return chunk, response, nil
}
