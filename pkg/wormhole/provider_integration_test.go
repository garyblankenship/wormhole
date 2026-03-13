package wormhole_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/internal/testutil"
	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_MultipleProviders(t *testing.T) {
	// Register test models in the global model registry
	testutil.SetupTestModels(t)

	t.Run("switching between providers", func(t *testing.T) {
		// OpenAI server
		openaiServer := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			response := map[string]any{
				"id":      "chatcmpl-openai123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "OpenAI response",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     5,
					"completion_tokens": 3,
					"total_tokens":      8,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		})

		// Anthropic-style server (different format)
		anthropicServer := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			response := map[string]any{
				"id":    "msg-anthropic123",
				"type":  "message",
				"model": "claude-3-opus",
				"content": []map[string]any{
					{
						"type": "text",
						"text": "Anthropic response",
					},
				},
				"stop_reason": "end_turn",
				"usage": map[string]any{
					"input_tokens":  5,
					"output_tokens": 3,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
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
	testutil.SetupTestModels(t)

	t.Run("metrics middleware", func(t *testing.T) {
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			response := map[string]any{
				"id":      "chatcmpl-middleware123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "Middleware test response",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     10,
					"completion_tokens": 5,
					"total_tokens":      15,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
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
		var capturedRequest any
		var capturedResponse any

		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			response := map[string]any{
				"id":      "chatcmpl-capture123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "Capture test response",
						},
						"finish_reason": "stop",
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		})

		// Create custom middleware that captures request/response
		captureMiddleware := func(next middleware.Handler) middleware.Handler {
			return func(ctx context.Context, req any) (any, error) {
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
	// Note: Don't call setupTestModels(t) here because OpenRouter tests depend on
	// auto-registered OpenRouter models from New() which setupTestModels() would clear

	t.Run("openrouter with gpt-5-mini", func(t *testing.T) {
		// Mock OpenRouter server (uses OpenAI-compatible format)
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Verify request headers
			assert.Equal(t, "Bearer test-openrouter-key", r.Header.Get("Authorization"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			// Read and verify request body
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]any
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
			messages := req["messages"].([]any)
			assert.Len(t, messages, 1)
			message := messages[0].(map[string]any)
			assert.Equal(t, "user", message["role"])
			assert.Equal(t, "Hello OpenRouter!", message["content"])

			// Mock OpenRouter response (OpenAI-compatible format)
			response := map[string]any{
				"id":      "chatcmpl-openrouter123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "openai/gpt-5-mini",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "Hello! I'm GPT-5 Mini via OpenRouter. How can I help you today?",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     5,
					"completion_tokens": 15,
					"total_tokens":      20,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
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

	t.Run("openrouter provider handles any model", func(t *testing.T) {
		// Create client with OpenRouter
		client := wormhole.New(
			wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
				APIKey: "test-key",
			}),
		)

		// NEW APPROACH: No model pre-registration needed
		// The provider handles model validation at request time
		// This test just verifies the client was created successfully
		assert.NotNil(t, client)

		// The beauty of the new architecture: any model name can be passed
		// and the provider will validate it when the request is made
		ctx := context.Background()

		// This will fail due to auth, but proves the model isn't pre-validated
		_, err := client.Text().
			Using("openrouter").
			Model("any-model-name-works"). // No pre-registration needed!
			Prompt("test").
			MaxTokens(5).
			Generate(ctx)

		// Should get auth error, not model validation error
		assert.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "auth", "Should get auth error, not model validation error")
	})

	t.Run("openrouter timeout handling", func(t *testing.T) {
		// Mock slow OpenRouter server
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Simulate slow response (200ms delay)
			time.Sleep(200 * time.Millisecond)

			response := map[string]any{
				"id":      "chatcmpl-slow123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "openai/gpt-5-mini",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "Slow response from OpenRouter",
						},
						"finish_reason": "stop",
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		})

		t.Run("with explicit timeout should succeed", func(t *testing.T) {
			// User should configure appropriate timeout for their use case
			client := wormhole.New(
				wormhole.WithDefaultProvider("openrouter"),
				wormhole.WithOpenAICompatible("openrouter", server.URL, types.ProviderConfig{
					APIKey: "test-key",
				}),
				wormhole.WithTimeout(1*time.Second), // User-configured timeout
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
				wormhole.WithModelValidation(false),       // Disable model validation for this timeout test
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
