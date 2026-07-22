package wormhole

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v2/types"
)

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
	return e.executeWithTools(ctx, request, provider.Text, maxIterations)
}

// executeWithTools runs the tool loop through the supplied text handler. This
// lets builders apply provider middleware once and reuse the wrapped handler
// for the initial request and every continuation turn.
func (e *ToolExecutor) executeWithTools(
	ctx context.Context,
	request types.TextRequest,
	handler types.TextHandler,
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
		response, err := handler(ctx, currentRequest)
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

		// Build assistant message with the tool calls (for conversation history).
		// Thinking carries the provider's signed reasoning block so Anthropic
		// extended-thinking + tool_use round-trips don't hard-400 on replay.
		assistantMessage := &types.AssistantMessage{
			Content:   response.Text,
			ToolCalls: response.ToolCalls,
			Thinking:  response.Thinking,
		}

		currentRequest.Messages = append(currentRequest.Messages, assistantMessage)
		for _, toolResultMessage := range e.BuildToolResultMessages(toolResults) {
			currentRequest.Messages = append(currentRequest.Messages, toolResultMessage)
		}

		// Continue loop - will call provider again with updated messages
	}

	// Max iterations reached
	return nil, fmt.Errorf("max tool execution iterations (%d) reached without final response", maxIterations)
}
