package transform

import (
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/v2/types"
)

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
	return MapFinishReason(reason)
}

// Predefined configurations for common providers

// NewOpenAIStreamingTransformer creates a transformer configured for OpenAI
func NewOpenAIStreamingTransformer() *StreamingTransformer {
	return NewStreamingTransformer(StreamingConfig{
		TextFieldPath:       "choices.0.delta.content",
		ToolCallFieldPath:   "choices.0.delta.tool_calls",
		FinishReasonPath:    "choices.0.finish_reason",
		UsagePath:           "usage",
		IDPath:              "id",
		ModelPath:           "model",
		ThinkingPath:        "choices.0.delta.reasoning_content",
		RefusalPath:         "choices.0.delta.refusal",
		FinishReasonAdapter: MapFinishReason,
		UsageAdapter:        openAIStreamUsage,
		ReturnsBatch:        false,
		ChunkType:           "text_chunk",
	})
}

// openAIStreamUsage parses OpenAI streamed usage including the cached-token
// detail that parseDefaultUsage omits. OpenAI-specific: only the OpenAI
// transformer wires this adapter, so other providers keep parseDefaultUsage.
func openAIStreamUsage(data any) (*types.Usage, error) {
	m, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unsupported usage data type: %T", data)
	}
	usage := &types.Usage{}
	if v, ok := m["prompt_tokens"].(float64); ok {
		usage.PromptTokens = int(v)
	}
	if v, ok := m["completion_tokens"].(float64); ok {
		usage.CompletionTokens = int(v)
	}
	if v, ok := m["total_tokens"].(float64); ok {
		usage.TotalTokens = int(v)
	}
	if details, ok := m["prompt_tokens_details"].(map[string]any); ok {
		if cached, ok := details["cached_tokens"].(float64); ok {
			usage.CacheReadTokens = int(cached)
		}
	}
	if details, ok := m["completion_tokens_details"].(map[string]any); ok {
		if reasoning, ok := details["reasoning_tokens"].(float64); ok {
			usage.ReasoningTokens = int(reasoning)
		}
	}
	return usage, nil
}

// NewAnthropicStreamingTransformer creates a transformer configured for Anthropic
func NewAnthropicStreamingTransformer() *StreamingTransformer {
	return NewStreamingTransformer(StreamingConfig{
		// Anthropic uses event-based streaming, so paths depend on event type
		// This is a simplified configuration for basic text extraction
		TextFieldPath:       "delta.text",
		FinishReasonPath:    "delta.stop_reason",
		UsagePath:           "usage",
		FinishReasonAdapter: MapFinishReason,
		ReturnsBatch:        false,
		ChunkType:           "stream_chunk",
	})
}

// NewOllamaStreamingTransformer creates a transformer configured for Ollama
func NewOllamaStreamingTransformer() *StreamingTransformer {
	return NewStreamingTransformer(StreamingConfig{
		TextFieldPath:         "message.content",
		FinishReasonPath:      "done",
		ExtraFinishReasonPath: "done_reason",
		IDPath:                "",
		ModelPath:             "model",
		FinishReasonAdapter:   MapFinishReason,
		ReturnsBatch:          false,
		ChunkType:             "text_chunk",
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
