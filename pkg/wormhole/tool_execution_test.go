package wormhole

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test constants
const testResultKey = "result"

// mockToolProvider simulates a provider that returns tool calls
type mockToolProvider struct {
	callCount int
	responses []*types.TextResponse
}

func (m *mockToolProvider) Name() string {
	return "mock"
}

func (m *mockToolProvider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	if m.callCount >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses available")
	}

	response := m.responses[m.callCount]
	m.callCount++
	return response, nil
}

func (m *mockToolProvider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.StreamChunk, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockToolProvider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockToolProvider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockToolProvider) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockToolProvider) SpeechToText(ctx context.Context, request types.SpeechToTextRequest) (*types.SpeechToTextResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockToolProvider) TextToSpeech(ctx context.Context, request types.TextToSpeechRequest) (*types.TextToSpeechResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockToolProvider) Images(ctx context.Context, request types.ImageRequest) (*types.ImagesResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockToolProvider) GenerateImage(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockToolProvider) Close() error {
	return nil
}

// ==================== Tool Executor Tests ====================

func TestToolExecutor_ExecuteSingleTool(t *testing.T) {
	registry := NewToolRegistry()

	// Register a simple tool with map[string]any schema
	weatherTool := types.Tool{
		Type:        "function",
		Name:        "get_weather",
		Description: "Get weather for a city",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city": map[string]any{"type": "string"},
			},
			"required": []string{"city"},
		},
	}

	handler := func(ctx context.Context, args map[string]any) (any, error) {
		city := args["city"].(string)
		return map[string]any{
			"city":        city,
			"temperature": 72,
			"condition":   "sunny",
		}, nil
	}

	registry.Register("get_weather", types.NewToolDefinition(weatherTool, handler))

	executor := NewToolExecutor(registry)

	// Create a tool call with map arguments
	toolCall := types.ToolCall{
		ID:        "call_123",
		Type:      "function",
		Name:      "get_weather",
		Arguments: map[string]any{"city": "San Francisco"},
	}

	// Execute
	result := executor.Execute(context.Background(), toolCall)

	assert.Equal(t, "call_123", result.ToolCallID)
	assert.Empty(t, result.Error)

	// Result is now a map
	data, ok := result.Result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "San Francisco", data["city"])
	assert.Equal(t, 72, data["temperature"])
}

func TestToolExecutor_ExecuteToolNotFound(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry)

	toolCall := types.ToolCall{
		ID:   "call_123",
		Name: "nonexistent_tool",
	}

	result := executor.Execute(context.Background(), toolCall)

	assert.Equal(t, "call_123", result.ToolCallID)
	assert.Contains(t, result.Error, "not found in registry")
}

func TestToolExecutor_ExecuteToolWithError(t *testing.T) {
	registry := NewToolRegistry()

	tool := types.Tool{
		Type:        "function",
		Name:        "failing_tool",
		InputSchema: map[string]any{},
	}

	handler := func(ctx context.Context, args map[string]any) (any, error) {
		return nil, fmt.Errorf("tool execution failed")
	}

	registry.Register("failing_tool", types.NewToolDefinition(tool, handler))

	executor := NewToolExecutor(registry)

	toolCall := types.ToolCall{
		ID:        "call_123",
		Name:      "failing_tool",
		Arguments: map[string]any{},
	}

	result := executor.Execute(context.Background(), toolCall)

	assert.Equal(t, "call_123", result.ToolCallID)
	assert.Contains(t, result.Error, "tool execution failed")
}

func TestToolExecutor_ExecuteAll(t *testing.T) {
	registry := NewToolRegistry()

	// Register multiple tools
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("tool_%d", i)
		tool := types.Tool{
			Type:        "function",
			Name:        name,
			InputSchema: map[string]any{},
		}

		idx := i
		handler := func(ctx context.Context, args map[string]any) (any, error) {
			return map[string]any{"tool": idx}, nil
		}

		registry.Register(name, types.NewToolDefinition(tool, handler))
	}

	executor := NewToolExecutor(registry)

	// Create multiple tool calls
	var toolCalls []types.ToolCall
	for i := 1; i <= 3; i++ {
		toolCalls = append(toolCalls, types.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      fmt.Sprintf("tool_%d", i),
			Arguments: map[string]any{},
		})
	}

	// Execute all
	results := executor.ExecuteAll(context.Background(), toolCalls)

	assert.Len(t, results, 3)

	for i, result := range results {
		assert.Equal(t, fmt.Sprintf("call_%d", i+1), result.ToolCallID)
		assert.Empty(t, result.Error)
	}
}

func TestToolExecutor_BuildToolResultMessage(t *testing.T) {
	executor := NewToolExecutor(NewToolRegistry())

	results := []types.ToolResult{
		{
			ToolCallID: "call_1",
			Result:     map[string]any{"data": "success"},
		},
		{
			ToolCallID: "call_2",
			Error:      "execution failed",
		},
	}

	message := executor.BuildToolResultMessage(results)

	assert.Equal(t, types.RoleTool, message.GetRole())
	assert.Contains(t, message.Content, "call_1")
	assert.Contains(t, message.Content, "success")
	assert.Contains(t, message.Content, "call_2")
	assert.Contains(t, message.Content, "failed")
}

func TestToolExecutor_ExecuteWithTools_SingleRound(t *testing.T) {
	registry := NewToolRegistry()

	// Register a weather tool
	weatherTool := types.Tool{
		Type:        "function",
		Name:        "get_weather",
		Description: "Get weather",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city": map[string]any{"type": "string"},
			},
			"required": []string{"city"},
		},
	}

	handler := func(ctx context.Context, args map[string]any) (any, error) {
		return map[string]any{"temp": 72, "condition": "sunny"}, nil
	}

	registry.Register("get_weather", types.NewToolDefinition(weatherTool, handler))

	executor := NewToolExecutor(registry)

	// Create mock provider
	provider := &mockToolProvider{
		responses: []*types.TextResponse{
			// First call: model requests tool
			{
				Text: "",
				ToolCalls: []types.ToolCall{
					{
						ID:        "call_1",
						Type:      "function",
						Name:      "get_weather",
						Arguments: map[string]any{"city": "SF"},
					},
				},
			},
			// Second call: final response after tool execution
			{
				Text:      "The weather in SF is 72°F and sunny.",
				ToolCalls: nil,
			},
		},
	}

	request := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gpt-4",
		},
		Messages: []types.Message{
			types.NewUserMessage("What's the weather in SF?"),
		},
	}

	// Execute with tools
	response, err := executor.ExecuteWithTools(context.Background(), request, provider, 10)

	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "The weather in SF is 72°F and sunny.", response.Text)
	assert.Equal(t, 2, provider.callCount) // Should make 2 calls
}

func TestToolExecutor_ExecuteWithTools_MaxIterations(t *testing.T) {
	registry := NewToolRegistry()

	// Register a tool
	tool := types.Tool{
		Type:        "function",
		Name:        "test_tool",
		InputSchema: map[string]any{},
	}

	handler := func(ctx context.Context, args map[string]any) (any, error) {
		return map[string]any{testResultKey: "ok"}, nil
	}

	registry.Register("test_tool", types.NewToolDefinition(tool, handler))

	executor := NewToolExecutor(registry)

	// Create provider that always returns tool calls (infinite loop)
	provider := &mockToolProvider{
		responses: []*types.TextResponse{
			{
				ToolCalls: []types.ToolCall{
					{
						ID:        "call_1",
						Name:      "test_tool",
						Arguments: map[string]any{},
					},
				},
			},
			{
				ToolCalls: []types.ToolCall{
					{
						ID:        "call_2",
						Name:      "test_tool",
						Arguments: map[string]any{},
					},
				},
			},
			{
				ToolCalls: []types.ToolCall{
					{
						ID:        "call_3",
						Name:      "test_tool",
						Arguments: map[string]any{},
					},
				},
			},
		},
	}

	request := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gpt-4",
		},
		Messages: []types.Message{
			types.NewUserMessage("Test"),
		},
	}

	// Execute with max iterations = 2
	_, err := executor.ExecuteWithTools(context.Background(), request, provider, 2)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max tool execution iterations")
	assert.Equal(t, 2, provider.callCount) // Should stop at max iterations
}

func TestToolExecutor_ExecuteWithTools_NoTools(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry)

	// Provider returns response without tool calls
	provider := &mockToolProvider{
		responses: []*types.TextResponse{
			{
				Text:      "Hello, world!",
				ToolCalls: nil,
			},
		},
	}

	request := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gpt-4",
		},
		Messages: []types.Message{
			types.NewUserMessage("Hello"),
		},
	}

	response, err := executor.ExecuteWithTools(context.Background(), request, provider, 10)

	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", response.Text)
	assert.Equal(t, 1, provider.callCount) // Should only make 1 call
}

// ==================== Integration Tests ====================

func TestWormhole_RegisterAndListTools(t *testing.T) {
	client := New()

	// Initially empty
	assert.Equal(t, 0, client.ToolCount())

	// Register a tool with map[string]any schema
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{"type": "string"},
		},
	}

	client.RegisterTool(
		"test_tool",
		"A test tool",
		schema,
		func(ctx context.Context, args map[string]any) (any, error) {
			return testResultKey, nil
		},
	)

	// Verify registration
	assert.Equal(t, 1, client.ToolCount())
	assert.True(t, client.HasTool("test_tool"))
	assert.False(t, client.HasTool("nonexistent"))

	tools := client.ListTools()
	assert.Len(t, tools, 1)
	assert.Equal(t, "test_tool", tools[0].Name)
	assert.Equal(t, "A test tool", tools[0].Description)
}

func TestWormhole_UnregisterAndClearTools(t *testing.T) {
	client := New()

	// Register tools
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("tool_%d", i)
		client.RegisterTool(
			name,
			"Test tool",
			map[string]any{"type": "string"},
			func(ctx context.Context, args map[string]any) (any, error) {
				return nil, nil
			},
		)
	}

	assert.Equal(t, 3, client.ToolCount())

	// Unregister one
	err := client.UnregisterTool("tool_1")
	assert.NoError(t, err)
	assert.Equal(t, 2, client.ToolCount())

	// Clear all
	client.ClearTools()
	assert.Equal(t, 0, client.ToolCount())
}

// ==================== Tool Safety Tests ====================

func TestToolExecutor_WithSafetyConfig(t *testing.T) {
	registry := NewToolRegistry()

	// Register a tool
	tool := types.Tool{
		Type:        "function",
		Name:        "test_tool",
		InputSchema: map[string]any{},
	}

	handler := func(ctx context.Context, args map[string]any) (any, error) {
		return map[string]any{"result": "ok"}, nil
	}

	registry.Register("test_tool", types.NewToolDefinition(tool, handler))

	// Create executor with custom safety config
	config := DefaultToolSafetyConfig()
	config.MaxConcurrentTools = 2 // Limit to 2 concurrent tools
	config.ToolTimeout = 1 * time.Second

	executor := NewToolExecutorWithConfig(registry, config)

	// Create multiple tool calls
	var toolCalls []types.ToolCall
	for i := 1; i <= 5; i++ {
		toolCalls = append(toolCalls, types.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      "test_tool",
			Arguments: map[string]any{},
		})
	}

	// Execute all - should respect concurrency limit
	results := executor.ExecuteAll(context.Background(), toolCalls)

	assert.Len(t, results, 5)
	for i, result := range results {
		assert.Equal(t, fmt.Sprintf("call_%d", i+1), result.ToolCallID)
		assert.Empty(t, result.Error)
	}
}

func TestToolExecutor_CircuitBreaker(t *testing.T) {
	registry := NewToolRegistry()

	// Register a failing tool
	tool := types.Tool{
		Type:        "function",
		Name:        "failing_tool",
		InputSchema: map[string]any{},
	}

	failureCount := 0
	handler := func(ctx context.Context, args map[string]any) (any, error) {
		failureCount++
		return nil, fmt.Errorf("intentional failure %d", failureCount)
	}

	registry.Register("failing_tool", types.NewToolDefinition(tool, handler))

	// Create executor with circuit breaker enabled
	config := DefaultToolSafetyConfig()
	config.EnableCircuitBreaker = true
	config.CircuitBreakerThreshold = 3 // Trip after 3 failures
	config.CircuitBreakerResetTimeout = 100 * time.Millisecond

	executor := NewToolExecutorWithConfig(registry, config)

	// Execute enough times to trip the circuit breaker
	var results []types.ToolResult
	for i := 1; i <= 5; i++ {
		toolCall := types.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      "failing_tool",
			Arguments: map[string]any{},
		}
		result := executor.Execute(context.Background(), toolCall)
		results = append(results, result)
	}

	// First 3 should fail with tool errors, next 2 should fail with circuit breaker error
	assert.Len(t, results, 5)
	for i := 0; i < 3; i++ {
		assert.Contains(t, results[i].Error, "intentional failure")
	}
	// Last 2 should be circuit breaker errors
	for i := 3; i < 5; i++ {
		assert.Contains(t, results[i].Error, "circuit breaker tripped")
	}

	// Wait for circuit breaker to reset
	time.Sleep(150 * time.Millisecond)

	// Try again - circuit breaker should have reset
	toolCall := types.ToolCall{
		ID:        "call_reset",
		Name:      "failing_tool",
		Arguments: map[string]any{},
	}
	result := executor.Execute(context.Background(), toolCall)
	assert.Contains(t, result.Error, "intentional failure")
}

func TestToolExecutor_CircuitBreakerHalfOpenRecovery(t *testing.T) {
	registry := NewToolRegistry()

	// Register a tool that fails first, then succeeds
	callCount := 0
	tool := types.Tool{
		Type:        "function",
		Name:        "recovering_tool",
		InputSchema: map[string]any{},
	}

	handler := func(ctx context.Context, args map[string]any) (any, error) {
		callCount++
		if callCount <= 3 {
			return nil, fmt.Errorf("failure %d", callCount)
		}
		return map[string]any{"success": true}, nil
	}

	registry.Register("recovering_tool", types.NewToolDefinition(tool, handler))

	// Create executor with circuit breaker enabled
	config := DefaultToolSafetyConfig()
	config.EnableCircuitBreaker = true
	config.CircuitBreakerThreshold = 3 // Trip after 3 failures
	config.CircuitBreakerResetTimeout = 100 * time.Millisecond

	executor := NewToolExecutorWithConfig(registry, config)

	// Execute 3 times to trip the circuit breaker
	for i := 1; i <= 3; i++ {
		toolCall := types.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      "recovering_tool",
			Arguments: map[string]any{},
		}
		result := executor.Execute(context.Background(), toolCall)
		assert.Contains(t, result.Error, "failure")
	}

	// Next call should be blocked by circuit breaker (open state)
	toolCall := types.ToolCall{
		ID:        "call_blocked",
		Name:      "recovering_tool",
		Arguments: map[string]any{},
	}
	result := executor.Execute(context.Background(), toolCall)
	assert.Contains(t, result.Error, "circuit breaker tripped")

	// Wait for circuit breaker to enter half-open state
	time.Sleep(150 * time.Millisecond)

	// Next call should be allowed (half-open state), and should succeed
	// This will transition circuit breaker back to closed state
	toolCall = types.ToolCall{
		ID:        "call_half_open",
		Name:      "recovering_tool",
		Arguments: map[string]any{},
	}
	result = executor.Execute(context.Background(), toolCall)
	assert.Empty(t, result.Error)
	assert.NotNil(t, result.Result)

	// Circuit should now be closed, subsequent calls should work
	toolCall = types.ToolCall{
		ID:        "call_closed",
		Name:      "recovering_tool",
		Arguments: map[string]any{},
	}
	result = executor.Execute(context.Background(), toolCall)
	assert.Empty(t, result.Error)
}

func TestToolExecutor_CircuitBreakerHalfOpenFailure(t *testing.T) {
	registry := NewToolRegistry()

	// Register a tool that always fails
	tool := types.Tool{
		Type:        "function",
		Name:        "failing_tool",
		InputSchema: map[string]any{},
	}

	handler := func(ctx context.Context, args map[string]any) (any, error) {
		return nil, fmt.Errorf("always fails")
	}

	registry.Register("failing_tool", types.NewToolDefinition(tool, handler))

	// Create executor with circuit breaker enabled, allow 2 test calls in half-open state
	config := DefaultToolSafetyConfig()
	config.EnableCircuitBreaker = true
	config.CircuitBreakerThreshold = 2 // Trip after 2 failures
	config.CircuitBreakerResetTimeout = 100 * time.Millisecond

	executor := NewToolExecutorWithConfig(registry, config)

	// Execute 2 times to trip the circuit breaker
	for i := 1; i <= 2; i++ {
		toolCall := types.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      "failing_tool",
			Arguments: map[string]any{},
		}
		result := executor.Execute(context.Background(), toolCall)
		assert.Contains(t, result.Error, "always fails")
	}

	// Next call should be blocked by circuit breaker (open state)
	toolCall := types.ToolCall{
		ID:        "call_blocked",
		Name:      "failing_tool",
		Arguments: map[string]any{},
	}
	result := executor.Execute(context.Background(), toolCall)
	assert.Contains(t, result.Error, "circuit breaker tripped")

	// Wait for circuit breaker to enter half-open state
	time.Sleep(150 * time.Millisecond)

	// Next call should be allowed (half-open state), but will fail
	// This should transition circuit breaker back to open state
	toolCall = types.ToolCall{
		ID:        "call_half_open",
		Name:      "failing_tool",
		Arguments: map[string]any{},
	}
	result = executor.Execute(context.Background(), toolCall)
	assert.Contains(t, result.Error, "always fails")

	// Circuit should now be open again, next call should be blocked
	toolCall = types.ToolCall{
		ID:        "call_reopened",
		Name:      "failing_tool",
		Arguments: map[string]any{},
	}
	result = executor.Execute(context.Background(), toolCall)
	assert.Contains(t, result.Error, "circuit breaker tripped")

	// Wait for another timeout period
	time.Sleep(150 * time.Millisecond)

	// Should be in half-open state again, allow test call
	toolCall = types.ToolCall{
		ID:        "call_half_open_again",
		Name:      "failing_tool",
		Arguments: map[string]any{},
	}
	result = executor.Execute(context.Background(), toolCall)
	assert.Contains(t, result.Error, "always fails")
}
