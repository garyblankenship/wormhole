package wormhole

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ToolExecutor handles the execution of tools and orchestration of multi-turn conversations
type ToolExecutor struct {
	registry *ToolRegistry
}

// NewToolExecutor creates a new ToolExecutor with the given registry
func NewToolExecutor(registry *ToolRegistry) *ToolExecutor {
	return &ToolExecutor{
		registry: registry,
	}
}

// Execute executes a single tool call and returns the result
//
// Parameters:
//   - ctx: Context for cancellation and timeout
//   - toolCall: The tool call from the LLM (contains name, ID, arguments)
//
// Returns:
//   - ToolResult with the execution result or error
func (e *ToolExecutor) Execute(ctx context.Context, toolCall types.ToolCall) types.ToolResult {
	// Get tool definition from registry
	definition := e.registry.Get(toolCall.Name)
	if definition == nil {
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      fmt.Sprintf("tool %q not found in registry", toolCall.Name),
		}
	}

	// Arguments are already a map from the provider
	args := toolCall.Arguments

	// Note: Tool.InputSchema is map[string]any, schema validation would require
	// reconstructing the Schema types from the map, which is complex.
	// For now, we rely on the provider to validate against the schema.

	// Execute the tool handler
	result, err := definition.Handler(ctx, args)
	if err != nil {
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      err.Error(),
		}
	}

	return types.ToolResult{
		ToolCallID: toolCall.ID,
		Result:     result, // Result is any, not string
	}
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

	for i, toolCall := range toolCalls {
		go func(idx int, tc types.ToolCall) {
			result := e.Execute(ctx, tc)
			resultChan <- resultWithIndex{index: idx, result: result}
		}(i, toolCall)
	}

	// Collect results
	for i := 0; i < len(toolCalls); i++ {
		r := <-resultChan
		results[r.index] = r.result
	}

	return results
}

// BuildToolResultMessage creates a ToolResultMessage from tool results
// This message is added to the conversation history to provide tool execution results back to the LLM
func (e *ToolExecutor) BuildToolResultMessage(toolResults []types.ToolResult) *types.ToolResultMessage {
	// For most providers, tool results are sent as a special message with role "tool"
	// The content depends on whether there are errors

	// Build content that includes all tool results
	var content string
	for i, result := range toolResults {
		if i > 0 {
			content += "\n"
		}

		if result.Error != "" {
			content += fmt.Sprintf("Tool %s failed: %s", result.ToolCallID, result.Error)
		} else {
			// Marshal result to JSON string for content
			resultJSON, err := json.Marshal(result.Result)
			if err != nil {
				content += fmt.Sprintf("Tool %s failed to serialize: %v", result.ToolCallID, err)
			} else {
				content += fmt.Sprintf("Tool %s result: %s", result.ToolCallID, string(resultJSON))
			}
		}
	}

	// Use the first tool call ID (providers may handle multiple results differently)
	toolCallID := ""
	if len(toolResults) > 0 {
		toolCallID = toolResults[0].ToolCallID
	}

	return &types.ToolResultMessage{
		Content:    content,
		ToolCallID: toolCallID,
	}
}

// ==================== Multi-Turn Orchestration ====================

// ExecuteWithTools orchestrates multi-turn conversations with automatic tool execution.
// It will:
//  1. Call the LLM with tools available
//  2. If tool calls are returned, execute them
//  3. Send results back to LLM
//  4. Repeat until no more tool calls (or max iterations reached)
//  5. Return the final text response
//
// Parameters:
//   - ctx: Context for cancellation
//   - request: The text request (should have tools set)
//   - provider: The provider to use for LLM calls
//   - maxIterations: Maximum number of tool execution rounds (default: 10)
//
// Returns:
//   - Final TextResponse after all tool executions
//   - Error if any step fails
func (e *ToolExecutor) ExecuteWithTools(
	ctx context.Context,
	request types.TextRequest,
	provider types.Provider,
	maxIterations int,
) (*types.TextResponse, error) {
	if maxIterations <= 0 {
		maxIterations = 10 // Default safety limit
	}

	// Make a copy of the request to avoid modifying the original
	currentRequest := request

	// Ensure tools are set in the request
	if len(currentRequest.Tools) == 0 {
		// Get all tools from registry
		currentRequest.Tools = e.registry.List()
	}

	iteration := 0
	for iteration < maxIterations {
		iteration++

		// Call the provider
		response, err := provider.Text(ctx, currentRequest)
		if err != nil {
			return nil, fmt.Errorf("provider call failed (iteration %d): %w", iteration, err)
		}

		// Check if there are tool calls
		if len(response.ToolCalls) == 0 {
			// No more tool calls - return final response
			return response, nil
		}

		// Execute all tool calls
		toolResults := e.ExecuteAll(ctx, response.ToolCalls)

		// Build assistant message with the tool calls (for conversation history)
		assistantMessage := &types.AssistantMessage{
			Content:   response.Text,
			ToolCalls: response.ToolCalls,
		}

		// Build tool result message
		toolResultMessage := e.BuildToolResultMessage(toolResults)

		// Add both messages to the conversation (they implement Message interface)
		currentRequest.Messages = append(currentRequest.Messages, assistantMessage, toolResultMessage)

		// Continue loop - will call provider again with updated messages
	}

	// Max iterations reached
	return nil, fmt.Errorf("max tool execution iterations (%d) reached without final response", maxIterations)
}
