package wormhole

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/garyblankenship/wormhole/v3/providers"
	"github.com/garyblankenship/wormhole/v3/types"
)

// validateOutputSize checks if the tool output exceeds configured size limits
func (e *ToolExecutor) validateOutputSize(result any) ([]byte, error) {

	// Measure the raw JSON bytes sent in tool-result messages.
	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize tool output: %w", err)
	}

	if e.safetyConfig.HasOutputSizeLimit() && len(jsonData) > e.safetyConfig.MaxToolOutputSize {
		return nil, fmt.Errorf("output size %d bytes exceeds limit of %d bytes", len(jsonData), e.safetyConfig.MaxToolOutputSize)
	}

	return jsonData, nil
}

// ExecuteAll executes all tool calls in parallel and returns the results
//
// Note: Tools are executed concurrently for performance. If you need sequential
// execution, call Execute() for each tool individually.
func (e *ToolExecutor) ExecuteAll(ctx context.Context, toolCalls []types.ToolCall) []types.ToolResult {
	results, _ := e.executeAll(ctx, toolCalls, false)
	return results
}

// executeAll retains the validated JSON only for the automatic message path.
// Public callers continue receiving typed results without retaining serialized copies.
func (e *ToolExecutor) executeAll(ctx context.Context, toolCalls []types.ToolCall, forMessages bool) ([]types.ToolResult, [][]byte) {
	if err := e.validateToolCallBatch(toolCalls); err != nil {
		return []types.ToolResult{{Error: err.Error()}}, nil
	}
	if len(toolCalls) == 0 {
		return nil, nil
	}
	results := make([]types.ToolResult, len(toolCalls))
	var encoded [][]byte
	if forMessages {
		encoded = make([][]byte, len(toolCalls))
	}

	workerCount := len(toolCalls)
	if max := e.safetyConfig.MaxConcurrentTools; max > 0 && workerCount > max {
		workerCount = max
	}
	var wg sync.WaitGroup
	var next atomic.Int64
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				idx := int(next.Add(1) - 1)
				if idx >= len(toolCalls) {
					return
				}
				if forMessages {
					result, _ := e.executeWithEncoding(ctx, toolCalls[idx], &encoded[idx])
					results[idx] = result
				} else {
					result, _ := e.execute(ctx, toolCalls[idx])
					results[idx] = result
				}
			}
		}()
	}
	wg.Wait()
	return results, encoded
}

func (e *ToolExecutor) validateToolCallBatch(toolCalls []types.ToolCall) error {
	if len(toolCalls) > e.safetyConfig.MaxToolCallsPerRound {
		return fmt.Errorf("tool call batch size %d exceeds limit of %d", len(toolCalls), e.safetyConfig.MaxToolCallsPerRound)
	}
	ids := make(map[string]struct{}, len(toolCalls))
	for _, call := range toolCalls {
		if call.ID == "" {
			return fmt.Errorf("tool call ID is empty")
		}
		normalized := providers.ToolCallIDSafePattern.ReplaceAllString(call.ID, "_")
		if len(normalized) > providers.ToolCallIDMaxLen {
			normalized = normalized[:providers.ToolCallIDMaxLen]
		}
		if _, duplicate := ids[normalized]; duplicate {
			return fmt.Errorf("duplicate normalized tool call ID %q", normalized)
		}
		ids[normalized] = struct{}{}
	}
	return nil
}
