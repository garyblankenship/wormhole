package wormhole

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/validation"
)

// ToolExecutor handles the execution of tools and orchestration of multi-turn conversations
type ToolExecutor struct {
	registry       *ToolRegistry
	safetyConfig   ToolSafetyConfig
	limiter        *ConcurrencyLimiter
	adaptiveLimiter *AdaptiveLimiter
	circuitBreaker *SimpleCircuitBreaker
	retryExecutor  *RetryExecutor
}

// NewToolExecutor creates a new ToolExecutor with the given registry and default safety config
func NewToolExecutor(registry *ToolRegistry) *ToolExecutor {
	return NewToolExecutorWithConfig(registry, DefaultToolSafetyConfig())
}

// NewToolExecutorWithConfig creates a new ToolExecutor with custom safety configuration
func NewToolExecutorWithConfig(registry *ToolRegistry, config ToolSafetyConfig) *ToolExecutor {
	// Validate and apply defaults
	_ = config.Validate() // #nosec G104 - Validate always returns nil

	executor := &ToolExecutor{
		registry:     registry,
		safetyConfig: config,
	}

	// Initialize concurrency limiter if configured
	if config.EnableAdaptiveConcurrency && !config.IsUnlimitedConcurrency() {
		// Use adaptive concurrency control
		executor.adaptiveLimiter = NewAdaptiveLimiter(config.ToAdaptiveConfig())
	} else if !config.IsUnlimitedConcurrency() {
		// Use fixed concurrency limit
		executor.limiter = NewConcurrencyLimiter(config.MaxConcurrentTools)
	}

	// Initialize circuit breaker if enabled
	if config.EnableCircuitBreaker {
		executor.circuitBreaker = NewSimpleCircuitBreaker(
			config.CircuitBreakerThreshold,
			config.CircuitBreakerResetTimeout,
		)
	}

	// Initialize retry executor if configured
	if config.MaxRetriesPerTool > 0 {
		executor.retryExecutor = NewRetryExecutor(config.MaxRetriesPerTool)
	}

	return executor
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
	// Check circuit breaker if enabled
	if e.circuitBreaker != nil && e.circuitBreaker.IsTripped() {
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      "circuit breaker tripped - tool execution temporarily disabled",
		}
	}

	// Get tool definition from registry
	definition := e.registry.Get(toolCall.Name)
	if definition == nil {
		// Record failure for circuit breaker
		if e.circuitBreaker != nil {
			e.circuitBreaker.RecordFailure()
		}
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      fmt.Sprintf("tool %q not found in registry", toolCall.Name),
		}
	}

	// Arguments are already a map from the provider
	args := toolCall.Arguments

	// Validate arguments against schema if schema is provided
	if definition.Tool.InputSchema != nil {
		if err := validation.ValidateAgainstSchema(args, definition.Tool.InputSchema); err != nil {
			// Record failure for circuit breaker
			if e.circuitBreaker != nil {
				e.circuitBreaker.RecordFailure()
			}
			return types.ToolResult{
				ToolCallID: toolCall.ID,
				Error:      fmt.Sprintf("schema validation failed: %v", err),
			}
		}
	}

	// Apply timeout if configured
	var cancel context.CancelFunc
	if e.safetyConfig.HasTimeout() {
		ctx, cancel = context.WithTimeout(ctx, e.safetyConfig.ToolTimeout)
		defer cancel()
	}

	// Execute the tool handler with retry logic if configured
	var result any
	var err error

	if e.retryExecutor != nil {
		err = e.retryExecutor.ExecuteWithRetry(ctx, func(ctx context.Context) error {
			r, e := definition.Handler(ctx, args)
			if e != nil {
				return e
			}
			result = r
			return nil
		})
	} else {
		result, err = definition.Handler(ctx, args)
	}

	if err != nil {
		// Record failure for circuit breaker
		if e.circuitBreaker != nil {
			e.circuitBreaker.RecordFailure()
		}
		return types.ToolResult{
			ToolCallID: toolCall.ID,
			Error:      err.Error(),
		}
	}

	// Record success for circuit breaker
	if e.circuitBreaker != nil {
		e.circuitBreaker.RecordSuccess()
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

	// Wait group to track goroutines
	var wg sync.WaitGroup

	for i, toolCall := range toolCalls {
		wg.Add(1)

		// Launch goroutine for each tool execution
		go func(idx int, tc types.ToolCall) {
			defer wg.Done()

			// Apply concurrency limiting if configured
			var startTime time.Time
			if e.adaptiveLimiter != nil {
				if !e.adaptiveLimiter.Acquire(ctx) {
					// Context cancelled or timeout while waiting for slot
					resultChan <- resultWithIndex{
						index: idx,
						result: types.ToolResult{
							ToolCallID: tc.ID,
							Error:      "concurrency limit exceeded or context cancelled",
						},
					}
					return
				}
				defer e.adaptiveLimiter.Release()
				startTime = time.Now()
			} else if e.limiter != nil {
				if !e.limiter.Acquire(ctx) {
					// Context cancelled or timeout while waiting for slot
					resultChan <- resultWithIndex{
						index: idx,
						result: types.ToolResult{
							ToolCallID: tc.ID,
							Error:      "concurrency limit exceeded or context cancelled",
						},
					}
					return
				}
				defer e.limiter.Release()
			}

			result := e.Execute(ctx, tc)

			// Record latency for adaptive concurrency control
			if !startTime.IsZero() {
				e.adaptiveLimiter.RecordLatency(time.Since(startTime))
			}

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

// BuildToolResultMessage creates a ToolResultMessage from tool results
// This message is added to the conversation history to provide tool execution results back to the LLM
func (e *ToolExecutor) BuildToolResultMessage(toolResults []types.ToolResult) *types.ToolResultMessage {
	// For most providers, tool results are sent as a special message with role "tool"
	// The content depends on whether there are errors

	// Build content that includes all tool results using strings.Builder for efficiency
	var builder strings.Builder
	for i, result := range toolResults {
		if i > 0 {
			builder.WriteString("\n")
		}

		if result.Error != "" {
			fmt.Fprintf(&builder, "Tool %s failed: %s", result.ToolCallID, result.Error)
		} else {
			// Marshal result to JSON string for content
			resultJSON, err := json.Marshal(result.Result)
			if err != nil {
				fmt.Fprintf(&builder, "Tool %s failed to serialize: %v", result.ToolCallID, err)
			} else {
				fmt.Fprintf(&builder, "Tool %s result: %s", result.ToolCallID, string(resultJSON))
			}
		}
	}
	content := builder.String()

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

// Stop stops any background goroutines used by the tool executor
func (e *ToolExecutor) Stop() {
	if e.adaptiveLimiter != nil {
		e.adaptiveLimiter.Stop()
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
