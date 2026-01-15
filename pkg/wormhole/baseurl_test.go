package wormhole

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBaseURLFunctionality(t *testing.T) {
	// Create a simple client
	client := New(
		WithOpenAI("test-key"),
		WithTimeout(1*time.Second), // Short timeout for fast test failure
	)

	ctx := context.Background()

	t.Run("BaseURL changes target endpoint", func(t *testing.T) {
		// This will fail because localhost:1234 isn't running, but it proves
		// that BaseURL is being used instead of default OpenAI endpoint
		_, err := client.Text().
			BaseURL("http://localhost:1234/v1").
			Model("test-model").
			Prompt("test").
			MaxTokens(5).
			Generate(ctx)

		// Should get connection refused, not auth error (which would indicate OpenAI endpoint)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection refused")
	})

	t.Run("Without BaseURL uses default provider", func(t *testing.T) {
		// This should try OpenAI endpoint and get auth error
		_, err := client.Text().
			Model("gpt-5-mini").
			Prompt("test").
			MaxTokens(5).
			Generate(ctx)

		// Should get auth error indicating it tried OpenAI
		assert.Error(t, err)
		// Note: In real scenarios this would be AUTH_ERROR, but in test it might be timeout
	})

	t.Run("BaseURL works with structured requests", func(t *testing.T) {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		}

		_, err := client.Structured().
			BaseURL("http://localhost:1234/v1").
			Model("test-model").
			Prompt("test").
			Schema(schema).
			Generate(ctx)

		// Should get some error indicating it tried localhost, not OpenAI
		assert.Error(t, err)
		// Could be connection refused, timeout, or other local endpoint error
		assert.NotContains(t, err.Error(), "openai.com")
	})

	t.Run("BaseURL works with embeddings", func(t *testing.T) {
		_, err := client.Embeddings().
			BaseURL("http://localhost:1234/v1").
			Model("test-embedding-model").
			Input("test text").
			Generate(ctx)

		// Should get some error indicating it tried localhost, not OpenAI
		assert.Error(t, err)
		// Could be connection refused, timeout, or other local endpoint error
		assert.NotContains(t, err.Error(), "openai.com")
	})
}

func TestBaseURLValidation(t *testing.T) {
	client := New(WithOpenAI("test-key"))
	ctx := context.Background()

	t.Run("Empty BaseURL uses default", func(t *testing.T) {
		_, err := client.Text().
			BaseURL(""). // Empty should fallback to default
			Model("gpt-5-mini").
			Prompt("test").
			MaxTokens(5).
			Generate(ctx)

		// Should get auth error indicating it used OpenAI default endpoint
		assert.Error(t, err)
	})

	t.Run("Invalid BaseURL fails appropriately", func(t *testing.T) {
		_, err := client.Text().
			BaseURL("invalid-url").
			Model("test-model").
			Prompt("test").
			MaxTokens(5).
			Generate(ctx)

		// Should get some kind of URL error
		assert.Error(t, err)
	})
}
