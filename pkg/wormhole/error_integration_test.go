package wormhole_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/internal/testutil"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ErrorHandling(t *testing.T) {
	// Register test models in the global model registry
	testutil.SetupTestModels(t)

	t.Run("HTTP 401 unauthorized", func(t *testing.T) {
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "Invalid API key",
					"type":    "invalid_request_error",
					"code":    "invalid_api_key",
				},
			})
		})

		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithOpenAICompatible("openai", server.URL, types.ProviderConfig{
				APIKey: "sk-invalid1234567890", // Valid format but invalid key
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
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "Rate limit exceeded",
					"type":    "rate_limit_error",
				},
			})
		})

		// Create client with NO retries to avoid hanging
		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithOpenAICompatible("openai", server.URL, types.ProviderConfig{
				APIKey:     "test-key",
				MaxRetries: func() *int { i := 0; return &i }(), // No retries
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
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "Internal server error",
					"type":    "server_error",
				},
			})
		})

		// Disable retries for this test to get immediate error
		noRetries := 0
		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:     "test-key",
				BaseURL:    server.URL,
				MaxRetries: &noRetries,
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
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
			// Simulate slow response that will trigger timeout
			time.Sleep(2 * time.Second)

			// Return a proper JSON response (which shouldn't be reached due to timeout)
			response := map[string]any{
				"id":      "chatcmpl-timeout123",
				"object":  "chat.completion",
				"created": 1699999999,
				"model":   "gpt-5",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "This shouldn't be reached",
						},
						"finish_reason": "stop",
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		})

		// Disable retries for this test to get immediate timeout error
		noRetries := 0
		client := wormhole.New(
			wormhole.WithDefaultProvider("openai"),
			wormhole.WithProviderConfig("openai", types.ProviderConfig{
				APIKey:     "test-key",
				BaseURL:    server.URL,
				Timeout:    1, // 1 second timeout (2s server sleep should trigger this)
				MaxRetries: &noRetries,
			}),
		)

		// Use context with timeout to ensure test doesn't hang
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := client.Text().
			Model("gpt-5").
			Prompt("Hello").
			Generate(ctx)

		require.Error(t, err)

		// Verify timeout error - may be wrapped by retry logic
		wormholeErr, ok := types.AsWormholeError(err)
		require.True(t, ok, "Expected WormholeError, got: %v (type: %T)", err, err)
		assert.Equal(t, types.ErrorCodeTimeout, wormholeErr.Code)
		assert.Contains(t, strings.ToLower(wormholeErr.Message), "timeout")
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := testutil.MockOpenAIServer(t, func(w http.ResponseWriter, r *http.Request) {
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
