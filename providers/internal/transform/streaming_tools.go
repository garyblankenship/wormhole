package transform

import (
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/v2/types"
)

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
func (t *StreamingTransformer) parseDefaultToolCalls(data any) []types.ToolCall {
	var toolCalls []types.ToolCall

	switch v := data.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				tc := t.parseToolCallFromMap(m)
				if tc != nil {
					toolCalls = append(toolCalls, *tc)
				}
			}
		}
	case map[string]any:
		tc := t.parseToolCallFromMap(v)
		if tc != nil {
			toolCalls = append(toolCalls, *tc)
		}
	default:
		// Try to unmarshal JSON string
		if str, ok := data.(string); ok {
			var arr []map[string]any
			if err := json.Unmarshal([]byte(str), &arr); err == nil {
				for _, m := range arr {
					tc := t.parseToolCallFromMap(m)
					if tc != nil {
						toolCalls = append(toolCalls, *tc)
					}
				}
			}
		}
	}

	return toolCalls
}

// parseToolCallFromMap parses a tool call from a map
func (t *StreamingTransformer) parseToolCallFromMap(m map[string]any) *types.ToolCall {
	tc := &types.ToolCall{}

	if idx, ok := m["index"].(float64); ok {
		tc.Index = int(idx)
	}
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

	return tc
}
