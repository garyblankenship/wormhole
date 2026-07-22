package wormhole

import (
	"context"
	"fmt"
	"sync"

	"github.com/garyblankenship/wormhole/v2/internal/pool"
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
	results := make([]types.ToolResult, len(toolCalls))

	// Execute all tools concurrently
	type resultWithIndex struct {
		index  int
		result types.ToolResult
	}

	resultChan := make(chan resultWithIndex, len(toolCalls))

	// Wait group to track goroutines
	var wg sync.WaitGroup

	for i, toolCall := range toolCalls {
		wg.Add(1)

		// Launch goroutine for each tool execution
		go func(idx int, tc types.ToolCall) {
			defer wg.Done()

			result := e.Execute(ctx, tc)
			result.Name = tc.Name

			resultChan <- resultWithIndex{index: idx, result: result}
		}(i, toolCall)
	}

	// Close result channel after all goroutines complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for r := range resultChan {
		results[r.index] = r.result
	}

	return results
}
