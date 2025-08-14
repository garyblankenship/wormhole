package wormhole_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockOpenAIServer creates a mock OpenAI API server for testing
func MockOpenAIServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)
	return server
}

// setupTestModels registers test models in the global model registry for testing
func setupTestModels(t *testing.T) {
	// Save original registry for cleanup
	originalRegistry := types.DefaultModelRegistry

	// Reset to empty registry for testing
	types.DefaultModelRegistry = types.NewModelRegistry()

	// Cleanup after test
	t.Cleanup(func() {
		types.DefaultModelRegistry = originalRegistry
	})

	// Register test models
	testModels := []*types.ModelInfo{
		{
			ID:          "gpt-5",
			Name:        "GPT-5",
			Provider:    "openai",
			Description: "Test GPT-5 model",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
			ContextLength: 128000,
			MaxTokens:     4096,
		},
		{
			ID:          "claude-3-opus",
			Name:        "Claude 3 Opus",
			Provider:    "anthropic",
			Description: "Test Claude 3 Opus model",
			Capabilities: []types.ModelCapability{
				types.CapabilityText,
				types.CapabilityChat,
				types.CapabilityFunctions,
				types.CapabilityStream,
			},
			ContextLength: 200000,
			MaxTokens:     4096,
		},
	}

	for _, model := range testModels {
		types.DefaultModelRegistry.Register(model)
	}
}

func TestOpenAIIntegration_TextGeneration(t *testing.T) {
	// Register test models in the global model registry
	setupTestModels(t)

	t.Run("successful text generation", func(t *testing.T) {
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Verify request method and path
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/chat/completions", r.URL.Path)

			// Verify headers
			assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			// Parse and verify request body
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]interface{}
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			assert.Equal(t, "gpt-5", req["model"])
			assert.Equal(t, 0.7, req["temperature"])
			assert.Equal(t, float64(100), req["max_tokens"])

			// Verify messages structure
			messages, ok := req["messages"].([]interface{})
			require.True(t, ok)
			require.Len(t, messages, 1)

			message := messages[0].(map[string]interface{})
			assert.Equal(t, "user", message["role"])
			assert.Equal(t, "Hello, how are you?", message["content"])

			// Send mock response
			response := map[string]interface{}{
				"id":      "chatcmpl-test123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "I'm doing well, thank you for asking!",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     10,
					"completion_tokens": 12,
					"total_tokens":      22,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		// Create Wormhole client with mock server
		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		// Execute request
		response, err := client.Text().
			Model("gpt-5").
			Prompt("Hello, how are you?").
			Temperature(0.7).
			MaxTokens(100).
			Generate(context.Background())

		// Verify response
		require.NoError(t, err)
		assert.Equal(t, "chatcmpl-test123", response.ID)
		assert.Equal(t, "gpt-5", response.Model)
		assert.Equal(t, "I'm doing well, thank you for asking!", response.Text)
		assert.Equal(t, types.FinishReasonStop, response.FinishReason)
		assert.Equal(t, 10, response.Usage.PromptTokens)
		assert.Equal(t, 12, response.Usage.CompletionTokens)
		assert.Equal(t, 22, response.Usage.TotalTokens)
	})

	t.Run("with system message", func(t *testing.T) {
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]interface{}
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			// Verify messages include system message
			messages := req["messages"].([]interface{})
			require.Len(t, messages, 2)

			systemMsg := messages[0].(map[string]interface{})
			assert.Equal(t, "system", systemMsg["role"])
			assert.Equal(t, "You are a helpful assistant.", systemMsg["content"])

			userMsg := messages[1].(map[string]interface{})
			assert.Equal(t, "user", userMsg["role"])
			assert.Equal(t, "What's 2+2?", userMsg["content"])

			// Mock response
			response := map[string]interface{}{
				"id":      "chatcmpl-math123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "2 + 2 = 4",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     15,
					"completion_tokens": 8,
					"total_tokens":      23,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		response, err := client.Text().
			Model("gpt-5").
			SystemPrompt("You are a helpful assistant.").
			Prompt("What's 2+2?").
			Generate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "2 + 2 = 4", response.Text)
	})

	t.Run("with multiple messages", func(t *testing.T) {
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]interface{}
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			// Verify conversation history
			messages := req["messages"].([]interface{})
			require.Len(t, messages, 3)

			assert.Equal(t, "user", messages[0].(map[string]interface{})["role"])
			assert.Equal(t, "Hello", messages[0].(map[string]interface{})["content"])

			assert.Equal(t, "assistant", messages[1].(map[string]interface{})["role"])
			assert.Equal(t, "Hi there!", messages[1].(map[string]interface{})["content"])

			assert.Equal(t, "user", messages[2].(map[string]interface{})["role"])
			assert.Equal(t, "How are you?", messages[2].(map[string]interface{})["content"])

			// Mock response
			response := map[string]interface{}{
				"id":      "chatcmpl-conv123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "I'm doing great, thanks for asking!",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     20,
					"completion_tokens": 10,
					"total_tokens":      30,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		messages := []types.Message{
			types.NewUserMessage("Hello"),
			types.NewAssistantMessage("Hi there!"),
			types.NewUserMessage("How are you?"),
		}

		response, err := client.Text().
			Model("gpt-5").
			Messages(messages...).
			Generate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "I'm doing great, thanks for asking!", response.Text)
		assert.Equal(t, 30, response.Usage.TotalTokens)
	})
}

func TestOpenAIIntegration_FunctionCalling(t *testing.T) {
	// Register test models in the global model registry
	setupTestModels(t)

	t.Run("function call request and response", func(t *testing.T) {
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]interface{}
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			// Verify tools in request
			tools, ok := req["tools"].([]interface{})
			require.True(t, ok)
			require.Len(t, tools, 1)

			tool := tools[0].(map[string]interface{})
			assert.Equal(t, "function", tool["type"])

			function := tool["function"].(map[string]interface{})
			assert.Equal(t, "get_weather", function["name"])
			assert.Equal(t, "Get current weather for a location", function["description"])

			// Verify tool_choice
			assert.Equal(t, "auto", req["tool_choice"])

			// Mock function call response
			response := map[string]interface{}{
				"id":      "chatcmpl-func123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role": "assistant",
							"tool_calls": []map[string]interface{}{
								{
									"id":   "call-123",
									"type": "function",
									"function": map[string]interface{}{
										"name":      "get_weather",
										"arguments": `{"location": "San Francisco, CA"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     25,
					"completion_tokens": 15,
					"total_tokens":      40,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		tool := types.NewTool(
			"get_weather",
			"Get current weather for a location",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"location": map[string]string{
						"type":        "string",
						"description": "The city and state, e.g. San Francisco, CA",
					},
				},
				"required": []string{"location"},
			},
		)

		response, err := client.Text().
			Model("gpt-5").
			Prompt("What's the weather like in San Francisco?").
			Tools(*tool).
			ToolChoice("auto").
			Generate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, types.FinishReasonToolCalls, response.FinishReason)
		require.Len(t, response.ToolCalls, 1)

		toolCall := response.ToolCalls[0]
		assert.Equal(t, "call-123", toolCall.ID)
		assert.Equal(t, "get_weather", toolCall.Function.Name)
		assert.Contains(t, toolCall.Function.Arguments, "San Francisco, CA")
	})
}

func TestOpenAIIntegration_StreamingGeneration(t *testing.T) {
	// Register test models in the global model registry
	setupTestModels(t)

	t.Run("streaming text generation", func(t *testing.T) {
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Verify streaming request
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]interface{}
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			assert.Equal(t, true, req["stream"])

			// Send SSE streaming response
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			flusher, ok := w.(http.Flusher)
			require.True(t, ok)

			// Send streaming chunks
			chunks := []string{
				`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-5","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-5","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-5","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-stream123","object":"chat.completion.chunk","created":1699999999,"model":"gpt-5","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				`data: [DONE]`,
			}

			for _, chunk := range chunks {
				_, err := w.Write([]byte(chunk + "\n\n"))
				require.NoError(t, err)
				flusher.Flush()
				time.Sleep(10 * time.Millisecond) // Simulate streaming delay
			}
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		stream, err := client.Text().
			Model("gpt-5").
			Prompt("Say hello").
			Stream(context.Background())

		require.NoError(t, err)

		// Collect streaming chunks
		var chunks []types.TextChunk
		for chunk := range stream {
			chunks = append(chunks, chunk)
			// Check for error in chunk
			if chunk.Error != nil {
				require.NoError(t, chunk.Error)
			}
		}

		// Verify we received the expected chunks
		require.Len(t, chunks, 4) // 3 content chunks + 1 finish chunk

		assert.Equal(t, "Hello", chunks[0].Text)
		assert.Equal(t, " there", chunks[1].Text)
		assert.Equal(t, "!", chunks[2].Text)

		// Check finish reason (should be pointer in chunk)
		require.NotNil(t, chunks[3].FinishReason)
		assert.Equal(t, types.FinishReasonStop, *chunks[3].FinishReason)

		// Verify full text concatenation
		fullText := ""
		for _, chunk := range chunks[:3] { // Exclude finish chunk
			fullText += chunk.Text
		}
		assert.Equal(t, "Hello there!", fullText)
	})
}

func TestIntegration_ErrorHandling(t *testing.T) {
	// Register test models in the global model registry
	setupTestModels(t)

	t.Run("HTTP 401 unauthorized", func(t *testing.T) {
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Invalid API key",
					"type":    "invalid_request_error",
					"code":    "invalid_api_key",
				},
			})
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "invalid-key",
				BaseURL: server.URL,
			}),
		)

		_, err := client.Text().
			Model("gpt-5").
			Prompt("Hello").
			Generate(context.Background())

		require.Error(t, err)

		// Verify error details
		wormholeErr, ok := types.AsWormholeError(err)
		require.True(t, ok)
		assert.Equal(t, types.ErrorCodeAuth, wormholeErr.Code)
		assert.Equal(t, "openai", wormholeErr.Provider)
		assert.Equal(t, 401, wormholeErr.StatusCode)
		assert.Contains(t, wormholeErr.Message, "Invalid API key")
	})

	t.Run("HTTP 429 rate limit", func(t *testing.T) {
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
				},
			})
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		_, err := client.Text().
			Model("gpt-5").
			Prompt("Hello").
			Generate(context.Background())

		require.Error(t, err)

		// Verify rate limit error
		wormholeErr, ok := types.AsWormholeError(err)
		require.True(t, ok)
		assert.Equal(t, types.ErrorCodeRateLimit, wormholeErr.Code)
		assert.Equal(t, 429, wormholeErr.StatusCode)
		assert.Contains(t, wormholeErr.Message, "Rate limit exceeded")
	})

	t.Run("HTTP 500 server error", func(t *testing.T) {
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Internal server error",
					"type":    "server_error",
				},
			})
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		_, err := client.Text().
			Model("gpt-5").
			Prompt("Hello").
			Generate(context.Background())

		require.Error(t, err)

		// Verify server error
		wormholeErr, ok := types.AsWormholeError(err)
		require.True(t, ok)
		assert.Equal(t, types.ErrorCodeProvider, wormholeErr.Code)
		assert.Equal(t, 500, wormholeErr.StatusCode)
		assert.Contains(t, wormholeErr.Message, "Internal server error")
	})

	t.Run("network timeout", func(t *testing.T) {
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Simulate slow response
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
				Timeout: 100, // 100ms timeout
			}),
		)

		_, err := client.Text().
			Model("gpt-5").
			Prompt("Hello").
			Generate(context.Background())

		require.Error(t, err)

		// Verify timeout error
		wormholeErr, ok := types.AsWormholeError(err)
		require.True(t, ok)
		assert.Equal(t, types.ErrorCodeTimeout, wormholeErr.Code)
		assert.Contains(t, strings.ToLower(wormholeErr.Message), "timeout")
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Simulate long response
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := client.Text().
			Model("gpt-5").
			Prompt("Hello").
			Generate(ctx)

		require.Error(t, err)
		assert.Equal(t, context.DeadlineExceeded, err)
	})
}

func TestIntegration_MultipleProviders(t *testing.T) {
	// Register test models in the global model registry
	setupTestModels(t)

	t.Run("switching between providers", func(t *testing.T) {
		// OpenAI server
		openaiServer := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{
				"id":      "chatcmpl-openai123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "OpenAI response",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     5,
					"completion_tokens": 3,
					"total_tokens":      8,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		// Anthropic-style server (different format)
		anthropicServer := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{
				"id":    "msg-anthropic123",
				"type":  "message",
				"model": "claude-3-opus",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": "Anthropic response",
					},
				},
				"stop_reason": "end_turn",
				"usage": map[string]interface{}{
					"input_tokens":  5,
					"output_tokens": 3,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: openaiServer.URL,
			}),
			wormhole.WithProviderConfig("anthropic", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: anthropicServer.URL,
			}),
		)

		// Test OpenAI provider
		response1, err := client.Text().
			Model("gpt-5").
			Prompt("Hello").
			Using("openai").
			Generate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "OpenAI response", response1.Text)
		assert.Equal(t, "chatcmpl-openai123", response1.ID)

		// Test Anthropic provider
		response2, err := client.Text().
			Model("claude-3-opus").
			Prompt("Hello").
			Using("anthropic").
			Generate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "Anthropic response", response2.Text)
		assert.Equal(t, "msg-anthropic123", response2.ID)
	})
}

func TestIntegration_Middleware(t *testing.T) {
	// Register test models in the global model registry
	setupTestModels(t)

	t.Run("metrics middleware", func(t *testing.T) {
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{
				"id":      "chatcmpl-middleware123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Middleware test response",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     10,
					"completion_tokens": 5,
					"total_tokens":      15,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		// Create metrics middleware
		metrics := middleware.NewMetrics()
		metricsMiddleware := middleware.MetricsMiddleware(metrics)

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
			wormhole.WithMiddleware(metricsMiddleware),
		)

		// Make a request
		response, err := client.Text().
			Model("gpt-5").
			Prompt("Test middleware").
			Generate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "Middleware test response", response.Text)

		// Verify metrics were recorded
		requests, errors, avgDuration := metrics.GetStats()
		assert.Equal(t, int64(1), requests)
		assert.Equal(t, int64(0), errors)
		assert.Greater(t, avgDuration, time.Duration(0))
	})

	t.Run("custom capture middleware", func(t *testing.T) {
		var capturedRequest interface{}
		var capturedResponse interface{}

		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{
				"id":      "chatcmpl-capture123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Capture test response",
						},
						"finish_reason": "stop",
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		// Create custom middleware that captures request/response
		captureMiddleware := func(next middleware.Handler) middleware.Handler {
			return func(ctx context.Context, req interface{}) (interface{}, error) {
				capturedRequest = req
				resp, err := next(ctx, req)
				if err == nil {
					capturedResponse = resp
				}
				return resp, err
			}
		}

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			}),
			wormhole.WithMiddleware(captureMiddleware),
		)

		response, err := client.Text().
			Model("gpt-5").
			Prompt("Test capture").
			Generate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, "Capture test response", response.Text)

		// Verify middleware captured the request and response
		assert.NotNil(t, capturedRequest)
		assert.NotNil(t, capturedResponse)

		// Type assert and verify request details
		textReq, ok := capturedRequest.(*types.TextRequest)
		require.True(t, ok)
		assert.Equal(t, "gpt-5", textReq.Model)
		assert.Len(t, textReq.Messages, 1)
		assert.Equal(t, "Test capture", textReq.Messages[0].GetContent())

		// Type assert and verify response details
		textResp, ok := capturedResponse.(*types.TextResponse)
		require.True(t, ok)
		assert.Equal(t, "chatcmpl-capture123", textResp.ID)
		assert.Equal(t, "Capture test response", textResp.Text)
	})
}

func TestIntegration_OpenRouter(t *testing.T) {
	// Register test models in the global model registry
	setupTestModels(t)

	t.Run("openrouter with gpt-5-mini", func(t *testing.T) {
		// Mock OpenRouter server (uses OpenAI-compatible format)
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Verify request headers
			assert.Equal(t, "Bearer test-openrouter-key", r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			// Read and verify request body
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]interface{}
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			// Verify OpenRouter request format
			assert.Equal(t, "openai/gpt-5-mini", req["model"])
			if maxTokens, ok := req["max_tokens"].(float64); ok {
				assert.Equal(t, 100, int(maxTokens))
			}
			if temp, ok := req["temperature"].(float64); ok {
				assert.Equal(t, 0.7, temp)
			}

			// Verify messages
			messages := req["messages"].([]interface{})
			assert.Len(t, messages, 1)
			message := messages[0].(map[string]interface{})
			assert.Equal(t, "user", message["role"])
			assert.Equal(t, "Hello OpenRouter!", message["content"])

			// Mock OpenRouter response (OpenAI-compatible format)
			response := map[string]interface{}{
				"id":      "chatcmpl-openrouter123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "openai/gpt-5-mini",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Hello! I'm GPT-5 Mini via OpenRouter. How can I help you today?",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     5,
					"completion_tokens": 15,
					"total_tokens":      20,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		// Create Wormhole client with OpenRouter provider
		client := wormhole.New(
			wormhole.WithDefaultProvider("openrouter"),
			wormhole.WithOpenAICompatible("openrouter", server.URL, types.ProviderConfig{
				APIKey: "test-openrouter-key",
			}),
		)

		// Execute request using OpenRouter provider
		response, err := client.Text().
			Model("openai/gpt-5-mini").
			Prompt("Hello OpenRouter!").
			Temperature(0.7).
			MaxTokens(100).
			Generate(context.Background())

		// Verify response
		require.NoError(t, err)
		assert.Equal(t, "chatcmpl-openrouter123", response.ID)
		assert.Equal(t, "openai/gpt-5-mini", response.Model)
		assert.Equal(t, "Hello! I'm GPT-5 Mini via OpenRouter. How can I help you today?", response.Text)
		assert.Equal(t, types.FinishReasonStop, response.FinishReason)
		assert.Equal(t, 5, response.Usage.PromptTokens)
		assert.Equal(t, 15, response.Usage.CompletionTokens)
		assert.Equal(t, 20, response.Usage.TotalTokens)
	})

	t.Run("openrouter model auto-registration", func(t *testing.T) {
		// Create client with OpenRouter - should auto-register models
		client := wormhole.New(
			wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
				APIKey: "test-key",
			}),
		)

		// Verify that popular OpenRouter models are auto-registered
		expectedModels := []string{
			"openai/gpt-5",
			"openai/gpt-5-mini", 
			"openai/gpt-5-nano",
			"anthropic/claude-opus-4",
			"anthropic/claude-sonnet-4",
			"google/gemini-2.5-pro",
			"google/gemini-2.5-flash",
			"mistralai/mistral-medium-3.1",
			"mistralai/codestral-2508",
		}

		for _, modelID := range expectedModels {
			modelInfo, exists := types.DefaultModelRegistry.Get(modelID)
			assert.True(t, exists, "Model %s should be auto-registered", modelID)
			if exists {
				assert.Equal(t, "openrouter", modelInfo.Provider)
				assert.Contains(t, modelInfo.Capabilities, types.CapabilityText)
			}
		}

		// Verify client was created successfully
		assert.NotNil(t, client)
	})

	t.Run("openrouter timeout handling", func(t *testing.T) {
		// Mock slow OpenRouter server
		server := MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Simulate slow response (200ms delay)
			time.Sleep(200 * time.Millisecond)
			
			response := map[string]interface{}{
				"id":      "chatcmpl-slow123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "openai/gpt-5-mini",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Slow response from OpenRouter",
						},
						"finish_reason": "stop",
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		})

		t.Run("with default timeout should succeed", func(t *testing.T) {
			// Default timeout should be sufficient for 200ms response
			client := wormhole.New(
				wormhole.WithDefaultProvider("openrouter"),
				wormhole.WithOpenAICompatible("openrouter", server.URL, types.ProviderConfig{
					APIKey: "test-key",
				}),
				wormhole.WithTimeout(1*time.Second), // Generous timeout
			)

			response, err := client.Text().
				Model("openai/gpt-5-mini").
				Prompt("Test timeout").
				Generate(context.Background())

			require.NoError(t, err)
			assert.Equal(t, "Slow response from OpenRouter", response.Text)
		})

		t.Run("with short timeout should fail gracefully", func(t *testing.T) {
			client := wormhole.New(
				wormhole.WithDefaultProvider("openrouter"),
				wormhole.WithOpenAICompatible("openrouter", server.URL, types.ProviderConfig{
					APIKey: "test-key",
				}),
				wormhole.WithTimeout(50*time.Millisecond), // Very short timeout
			)

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			_, err := client.Text().
				Model("openai/gpt-5-mini").
				Prompt("Test timeout").
				Generate(ctx)

			// Should get timeout error
			require.Error(t, err)
			errMsg := strings.ToLower(err.Error())
			assert.True(t, strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded"),
				"Expected timeout or deadline exceeded error, got: %s", err.Error())
		})
	})
}
