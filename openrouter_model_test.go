package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestOpenRouterModels(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	client := wormhole.New(
		wormhole.WithDefaultProvider("openrouter"),
		wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
			APIKey: apiKey,
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tests := []struct {
		name  string
		model string
		expectError bool
	}{
		{
			name:  "GPT-OSS-120B (target model)",
			model: "openai/gpt-oss-120b",
			expectError: true, // May not exist on OpenRouter despite being registered
		},
		{
			name:  "GPT-5",
			model: "openai/gpt-5",
			expectError: false,
		},
		{
			name:  "GPT-4o-mini (known working)",
			model: "openai/gpt-4o-mini",
			expectError: false,
		},
		{
			name:  "GPT-5-mini",
			model: "openai/gpt-5-mini",
			expectError: false,
		},
		{
			name:  "GPT-4.1-mini",
			model: "openai/gpt-4.1-mini",
			expectError: false, // Now registered
		},
		{
			name:  "GPT-4.1",
			model: "openai/gpt-4.1",
			expectError: false, // Now registered
		},
		{
			name:  "GPT-4o",
			model: "openai/gpt-4o",
			expectError: false,
		},
		{
			name:  "O3",
			model: "openai/o3",
			expectError: false, // Now registered
		},
		{
			name:  "GPT-3.5-turbo",
			model: "openai/gpt-3.5-turbo",
			expectError: false, // Now registered
		},
		{
			name:  "O1-mini",
			model: "openai/o1-mini",
			expectError: false, // Now registered
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := client.Text().
				Model(tt.model).
				Prompt("Hello! Please respond with just 'OK' to confirm you're working.").
				Generate(ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for model %s, but got success", tt.model)
				} else {
					t.Logf("Expected error for %s: %v", tt.model, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for model %s: %v", tt.model, err)
				} else {
					t.Logf("Success for %s: %s", tt.model, response.Text)
				}
			}
		})
	}
}

func TestOpenRouterProviderRouting(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	client := wormhole.New(
		wormhole.WithDefaultProvider("openrouter"),
		wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
			APIKey: apiKey,
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test that provider routing works correctly
	response, err := client.Text().
		Model("openai/gpt-4o-mini").
		Prompt("What provider are you running on?").
		Generate(ctx)

	if err != nil {
		t.Fatalf("Provider routing test failed: %v", err)
	}

	if response.Text == "" {
		t.Fatal("Empty response from provider routing test")
	}

	t.Logf("Provider routing test successful. Model: %s, Response: %s", response.Model, response.Text)
}

func TestOpenRouterModelAvailability(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	// Create OpenAI-compatible provider directly to test model listing
	provider := openai_compatible.NewGeneric("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
		APIKey: apiKey,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	models, err := provider.ListModels(ctx)
	if err != nil {
		t.Fatalf("Failed to list models: %v", err)
	}

	// Check if our target model exists
	targetModel := "openai/gpt-oss-120b"
	modelExists := false
	
	for _, model := range models.Data {
		if model.ID == targetModel {
			modelExists = true
			break
		}
		if model.ID == "gpt-oss-120b" { // In case it's without the openai/ prefix
			t.Logf("Found model without prefix: %s", model.ID)
		}
	}

	if !modelExists {
		t.Logf("Model %s not found in available models. First few models:", targetModel)
		for i, model := range models.Data {
			if i < 10 { // Show first 10 models
				t.Logf("  - %s (owned by %s)", model.ID, model.OwnedBy)
			}
		}
	} else {
		t.Logf("Model %s is available", targetModel)
	}
}

// Benchmark the difference between working and non-working models
func BenchmarkOpenRouterModels(b *testing.B) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		b.Skip("OPENROUTER_API_KEY not set")
	}

	client := wormhole.New(
		wormhole.WithDefaultProvider("openrouter"),
		wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
			APIKey: apiKey,
		}),
	)

	ctx := context.Background()

	b.Run("gpt-4o-mini", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := client.Text().
				Model("openai/gpt-4o-mini").
				Prompt("OK").
				Generate(ctx)
			if err != nil {
				b.Errorf("Request failed: %v", err)
			}
		}
	})

	b.Run("gpt-oss-120b", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := client.Text().
				Model("openai/gpt-oss-120b").
				Prompt("OK").
				Generate(ctx)
			// Expect this to fail, but measure how quickly it fails
			_ = err
		}
	})
}