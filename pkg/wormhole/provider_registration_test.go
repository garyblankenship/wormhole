package wormhole

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mockpkg "github.com/garyblankenship/wormhole/pkg/testing"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderRegistration(t *testing.T) {
	t.Parallel()
	t.Run("built-in providers are registered", func(t *testing.T) {
		t.Parallel()
		wormhole := New(WithOpenAI("test-key"))

		// Verify that core built-in providers are registered
		assert.Contains(t, wormhole.providerFactories, "openai")
		assert.Contains(t, wormhole.providerFactories, "anthropic")
		assert.Contains(t, wormhole.providerFactories, "gemini")
		assert.Contains(t, wormhole.providerFactories, "ollama")

		// groq and mistral are no longer built-in factories - they use WithOpenAICompatible()
		assert.NotContains(t, wormhole.providerFactories, "groq")
		assert.NotContains(t, wormhole.providerFactories, "mistral")
	})

	t.Run("custom provider registration", func(t *testing.T) {
		t.Parallel()
		// Register a custom provider via functional options
		customFactory := func(config types.ProviderConfig) (types.Provider, error) {
			return mockpkg.NewMockProvider("custom"), nil
		}

		wormhole := New(
			WithCustomProvider("custom", customFactory),
			WithProviderConfig("custom", types.ProviderConfig{APIKey: "test-key"}),
		)

		// Verify the custom provider is registered
		assert.Contains(t, wormhole.providerFactories, "custom")

		// Test that we can get the custom provider
		provider, err := wormhole.Provider("custom")
		require.NoError(t, err)
		assert.Equal(t, "custom", provider.Name())
	})

	t.Run("provider factory creates instances", func(t *testing.T) {
		t.Parallel()
		// Register a test provider factory with call counting
		callCount := 0
		testFactory := func(config types.ProviderConfig) (types.Provider, error) {
			callCount++
			return mockpkg.NewMockProvider("test"), nil
		}

		wormhole := New(
			WithCustomProvider("test", testFactory),
			WithProviderConfig("test", types.ProviderConfig{APIKey: "test-key"}),
		)

		// First call should create the provider
		provider1, err := wormhole.Provider("test")
		require.NoError(t, err)
		assert.Equal(t, "test", provider1.Name())
		assert.Equal(t, 1, callCount)

		// Second call should return cached provider (factory not called again)
		provider2, err := wormhole.Provider("test")
		require.NoError(t, err)
		assert.Equal(t, provider1, provider2) // Same instance
		assert.Equal(t, 1, callCount)         // Factory not called again
	})

	t.Run("unregistered provider returns error", func(t *testing.T) {
		t.Parallel()
		wormhole := New() // Empty client with no providers

		_, err := wormhole.Provider("nonexistent")
		assert.Error(t, err)
		// DX improvement: error now includes helpful hint about which providers are configured
		assert.Contains(t, err.Error(), "provider not configured")
		assert.Contains(t, err.Error(), "nonexistent")
	})

	t.Run("custom provider with auto-config works", func(t *testing.T) {
		t.Parallel()
		// WithCustomProvider automatically creates a config placeholder
		wormhole := New(
			WithCustomProvider("autoconfigured", func(config types.ProviderConfig) (types.Provider, error) {
				return mockpkg.NewMockProvider("autoconfigured"), nil
			}),
			// Note: WithCustomProvider auto-creates empty config
		)

		provider, err := wormhole.Provider("autoconfigured")
		assert.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, "autoconfigured", provider.Name())
	})
}

func TestWithOpenAICompatibleOption(t *testing.T) {
	t.Parallel()
	t.Run("WithOpenAICompatible option registers provider", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/chat/completions", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"chatcmpl-alias","created":100,"model":"alias-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
		}))
		t.Cleanup(server.Close)

		// Use WithOpenAICompatible option to add a provider during initialization
		wormhole := New(
			WithOpenAICompatible("custom-openai", server.URL, types.ProviderConfig{
				APIKey: "test-key",
			}),
			WithDiscovery(false),
		)

		// Verify the provider is registered and configured
		assert.Contains(t, wormhole.providerFactories, "custom-openai")
		assert.Contains(t, wormhole.config.Providers, "custom-openai")
		assert.Equal(t, server.URL, wormhole.config.Providers["custom-openai"].BaseURL)

		provider, err := wormhole.Provider("custom-openai")
		require.NoError(t, err)
		assert.Equal(t, "custom-openai", provider.Name())

		resp, err := wormhole.Text().
			Using("custom-openai").
			Model("alias-model").
			Prompt("hi").
			Generate(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "custom-openai", resp.Provider)
	})

	t.Run("WithGemini option stores config correctly", func(t *testing.T) {
		t.Parallel()
		// Use WithGemini option to add provider during initialization
		wormhole := New(
			WithGemini("test-api-key", types.ProviderConfig{
				BaseURL: "custom-base-url",
			}),
		)

		// Verify config is stored with API key
		assert.Contains(t, wormhole.config.Providers, "gemini")
		config := wormhole.config.Providers["gemini"]
		assert.Equal(t, "test-api-key", config.APIKey)
		assert.Equal(t, "custom-base-url", config.BaseURL)
	})

}

func TestWithLocalOpenAI(t *testing.T) {
	t.Run("configures no-auth local-compatible provider", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/chat/completions", r.URL.Path)
			assert.Empty(t, r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"chatcmpl-local","created":100,"model":"local-model","choices":[{"index":0,"message":{"role":"assistant","content":"local ok"},"finish_reason":"stop"}]}`)
		}))
		t.Cleanup(server.Close)

		client := New(
			WithLocalOpenAI(server.URL+"/v1"),
			WithDiscovery(false),
		)
		defer func() { _ = client.Close() }()

		config := client.config.Providers["local"]
		assert.True(t, config.NoAuth)
		assert.True(t, config.DynamicModels)
		require.NotNil(t, config.MaxRetries)
		assert.Equal(t, 0, *config.MaxRetries)
		assert.Equal(t, "local", client.config.DefaultProvider)

		resp, err := client.Text().
			Model("local-model").
			Prompt("hi").
			Generate(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "local ok", resp.Text)
		assert.Equal(t, "local", resp.Provider)
	})

	t.Run("disables retries by default", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			http.Error(w, "try later", http.StatusServiceUnavailable)
		}))
		t.Cleanup(server.Close)

		client := New(
			WithLocalOpenAI(server.URL+"/v1"),
			WithDiscovery(false),
		)
		defer func() { _ = client.Close() }()

		_, err := client.Text().
			Model("local-model").
			Prompt("hi").
			Generate(context.Background())
		require.Error(t, err)
		assert.Equal(t, 1, attempts)
	})
}

func TestOpenAIBaseURLValidationMode(t *testing.T) {
	t.Run("custom OpenAI base URL skips OpenAI key format validation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer local-key", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"chatcmpl-compatible","created":100,"model":"local-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
		}))
		t.Cleanup(server.Close)

		client := New(
			WithOpenAI("local-key", types.ProviderConfig{
				BaseURL: server.URL,
			}),
			WithDiscovery(false),
		)
		defer func() { _ = client.Close() }()

		resp, err := client.Text().
			Model("local-model").
			Prompt("hi").
			Generate(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Text)
	})

	t.Run("official OpenAI base URL keeps OpenAI key format validation", func(t *testing.T) {
		client := New(
			WithOpenAI("local-key", types.ProviderConfig{
				BaseURL: "https://api.openai.com/v1",
			}),
			WithDiscovery(false),
		)
		defer func() { _ = client.Close() }()

		_, err := client.Provider("openai")
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "invalid OpenAI API key format"), err.Error())
	})
}

func TestRunOpenAICompatibleSmoke(t *testing.T) {
	t.Run("validates chat request and response parse", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/chat/completions", r.URL.Path)
			assert.Empty(t, r.Header.Get("Authorization"))

			var body struct {
				Model    string `json:"model"`
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "local-model", body.Model)
			require.Len(t, body.Messages, 1)
			assert.Equal(t, "user", body.Messages[0].Role)
			assert.Equal(t, "ping", body.Messages[0].Content)

			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"chatcmpl-smoke","created":100,"model":"local-model","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}]}`)
		}))
		t.Cleanup(server.Close)

		result, err := RunOpenAICompatibleSmoke(context.Background(), OpenAICompatibleSmokeConfig{
			BaseURL: server.URL + "/v1",
			Model:   "local-model",
			Prompt:  "ping",
		})
		require.NoError(t, err)
		assert.Equal(t, "pong", result.Text)
		assert.Equal(t, "local-model", result.Model)
	})

	t.Run("validates required inputs", func(t *testing.T) {
		_, err := RunOpenAICompatibleSmoke(context.Background(), OpenAICompatibleSmokeConfig{Model: "local-model"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "base URL is required")

		_, err = RunOpenAICompatibleSmoke(context.Background(), OpenAICompatibleSmokeConfig{BaseURL: "http://localhost:8000/v1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model is required")
	})
}
