package wormhole

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/garyblankenship/wormhole/v2/internal/pool"
	"github.com/garyblankenship/wormhole/v2/providers"
	"github.com/garyblankenship/wormhole/v2/types"
)

// validateOutputSize checks if the tool output exceeds configured size limits
func (e *ToolExecutor) validateOutputSize(result any) error {
	if result == nil {
		return nil
	}

	// Try to estimate size by marshaling to JSON using pooled buffer
	jsonData, err := pool.Marshal(result)
	if err != nil {
		// If we can't marshal, we can't validate - log warning but allow
		// In production, you might want to handle this differently
		return nil
	}
	defer pool.Return(jsonData)

	if len(jsonData) > e.safetyConfig.MaxToolOutputSize {
		return fmt.Errorf("output size %d bytes exceeds limit of %d bytes", len(jsonData), e.safetyConfig.MaxToolOutputSize)
	}

	return nil
}

// ExecuteAll executes all tool calls in parallel and returns the results
//
// Note: Tools are executed concurrently for performance. If you need sequential
// execution, call Execute() for each tool individually.
func (e *ToolExecutor) ExecuteAll(ctx context.Context, toolCalls []types.ToolCall) []types.ToolResult {
	if err := e.validateToolCallBatch(toolCalls); err != nil {
		return []types.ToolResult{{Error: err.Error()}}
	}
	if len(toolCalls) == 0 {
		return nil
	}
	results := make([]types.ToolResult, len(toolCalls))
	for i := range results {
		results[i] = types.ToolResult{
			ToolCallID: toolCalls[i].ID,
			Name:       toolCalls[i].Name,
			Error:      "tool execution not started because an earlier handler outlived cancellation",
		}
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
				result, handlerRunning := e.execute(ctx, toolCalls[idx])
				result.Name = toolCalls[idx].Name
				results[idx] = result
				if handlerRunning {
					return
				}
			}
		}()
	}
	wg.Wait()
	return results
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
