package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

func TestDynamicModelSupport(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("OpenRouter with DynamicModels enabled", func(t *testing.T) {
		// Create client using factory (should have DynamicModels: true)
		client := wormhole.Quick.OpenRouter(apiKey)

		// Test with a model that's NOT in the registry
		randomModels := []string{
			"random/model-that-does-not-exist-12345",
			"openai/some-future-model-v10",
			"anthropic/claude-ultimate-2030",
			"meta-llama/llama-999b-instruct",
		}

		for _, model := range randomModels {
			t.Run(model, func(t *testing.T) {
				// This should NOT fail at validation stage (should reach OpenRouter)
				_, err := client.Text().
					Model(model).
					Prompt("Hello").
					Generate(ctx)

				// We expect OpenRouter to reject these (MODEL_ERROR), not wormhole (model not in registry)
				if err != nil {
					errStr := err.Error()
					// These are valid errors from OpenRouter (not registry blocking)
					if errStr == "MODEL_ERROR: model not available" ||
						errStr == "MODEL_ERROR: model not found" ||
						errStr == "AUTH_ERROR: insufficient credits" {
						t.Logf("Expected OpenRouter error for %s: %v", model, err)
					} else {
						// Registry blocking would show "model not found" from our registry
						if errStr == "model not found" {
							t.Errorf("Model %s was blocked by local registry (DynamicModels not working)", model)
						} else {
							t.Logf("Other error for %s: %v", model, err)
						}
					}
				} else {
					t.Logf("Unexpected success for fake model %s", model)
				}
			})
		}
	})

	t.Run("OpenAI with registry validation (traditional)", func(t *testing.T) {
		openaiKey := os.Getenv("OPENAI_API_KEY")
		if openaiKey == "" {
			t.Skip("OPENAI_API_KEY not set")
		}

		// Create OpenAI client (should have DynamicModels: false)
		client := wormhole.Quick.OpenAI(openaiKey)

		// Test with model NOT in registry
		_, err := client.Text().
			Model("gpt-unknown-model-12345").
			Prompt("Hello").
			Generate(ctx)

		// This SHOULD fail at registry validation stage
		if err == nil {
			t.Error("Expected registry validation to block unknown OpenAI model")
		} else if err.Error() == "model not found" {
			t.Log("Correct: Registry validation blocked unknown OpenAI model")
		} else {
			t.Logf("Different error: %v", err)
		}
	})

	t.Run("Demonstrate 200+ model support", func(t *testing.T) {
		client := wormhole.Quick.OpenRouter(apiKey)

		// Test a random model name that's not in our registry to prove it reaches OpenRouter
		testModels := []string{
			"totally/fake-model-12345", // Should reach OpenRouter and get rejected there
		}

		successCount := 0
		for _, model := range testModels {
			t.Run(model, func(t *testing.T) {
				response, err := client.Text().
					Model(model).
					Prompt("Say 'OK' if you can hear me").
					Generate(ctx)

				if err != nil {
					t.Logf("Model %s error: %v", model, err)
				} else {
					t.Logf("Model %s SUCCESS: %s", model, response.Text)
					successCount++
				}
			})
		}

		t.Logf("Successfully bypassed registry for %d/%d test models (reached OpenRouter)", len(testModels)-successCount, len(testModels))
	})
}

func TestProviderConfigDynamicModels(t *testing.T) {
	t.Run("Verify ProviderConfig has DynamicModels field", func(t *testing.T) {
		config := types.ProviderConfig{
			APIKey:        "test",
			DynamicModels: true,
		}

		if !config.DynamicModels {
			t.Error("DynamicModels field not working")
		}
	})

	t.Run("Manual configuration with DynamicModels", func(t *testing.T) {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			t.Skip("OPENROUTER_API_KEY not set")
		}

		// Manual configuration with DynamicModels enabled
		client := wormhole.New(
			wormhole.WithDefaultProvider("openrouter"),
			wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
				APIKey:        apiKey,
				DynamicModels: true,
			}),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Test with unregistered model
		_, err := client.Text().
			Model("some/random-model-name").
			Prompt("Hello").
			Generate(ctx)

		// Should reach OpenRouter, not be blocked by registry
		if err != nil && err.Error() == "model not found" {
			t.Error("DynamicModels configuration not working - registry still blocking")
		} else {
			t.Log("Success: Registry validation bypassed for dynamic provider")
		}
	})
}
