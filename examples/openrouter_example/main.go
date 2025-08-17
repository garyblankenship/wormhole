package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	// Get OpenRouter API key from environment
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY environment variable is required")
	}

	// 🚀 NEW APPROACH: Super simple BaseURL method (recommended)
	fmt.Println("🆕 NEW: Using BaseURL approach (simplest)")
	client := wormhole.New(
		wormhole.WithOpenAI(apiKey), // Just use OpenAI provider with your OpenRouter key
		wormhole.WithTimeout(2*time.Minute),
		wormhole.WithDebugLogging(),
	)

	// 🔄 LEGACY: Still works with WithOpenAICompatible (uses openai provider under hood)
	fmt.Println("🔄 LEGACY: WithOpenAICompatible still works")
	legacyClient := wormhole.New(
		wormhole.WithDefaultProvider("openrouter"),
		wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
			APIKey: apiKey,
		}),
		wormhole.WithTimeout(2*time.Minute),
	)

	// Use legacyClient for the rest of examples to show it still works
	_ = client // Both approaches work

	ctx := context.Background()

	fmt.Println("🌌 Wormhole + OpenRouter: Multi-Model Madness!")
	fmt.Println("===========================================")

	// Example 1: Basic text generation with different models
	fmt.Println("\n1. 🚀 Basic Text Generation (Multiple Models)")
	models := []string{
		"openai/gpt-4o-mini",               // OpenAI via OpenRouter
		"anthropic/claude-3.5-sonnet",      // Anthropic via OpenRouter
		"meta-llama/llama-3.1-8b-instruct", // Meta via OpenRouter
		"google/gemini-pro",                // Google via OpenRouter
		"mistralai/mixtral-8x7b-instruct",  // Mistral via OpenRouter
	}

	for _, model := range models {
		fmt.Printf("\n🧠 Testing model: %s\n", model)

		// NEW: Using BaseURL approach
		response, err := client.Text().
			BaseURL("https://openrouter.ai/api/v1"). // ✨ Just add this line!
			Model(model).
			Prompt("Explain quantum computing in one sentence.").
			MaxTokens(100).
			Temperature(0.7).
			Generate(ctx)

		if err != nil {
			fmt.Printf("❌ Error with %s: %v\n", model, err)
			continue
		}

		fmt.Printf("✅ Response: %s\n", response.Text)
	}

	// Example 2: Streaming with OpenRouter
	fmt.Println("\n\n2. 📡 Streaming Response")
	fmt.Printf("🧠 Model: openai/gpt-4o-mini (streaming)\n")

	stream, err := legacyClient.Text().
		Model("openai/gpt-4o-mini").
		Prompt("Write a haiku about dimensional travel").
		MaxTokens(100).
		Temperature(0.8).
		Stream(ctx)

	if err != nil {
		log.Fatalf("Failed to create stream: %v", err)
	}

	fmt.Print("✅ Streaming: ")
	for chunk := range stream {
		if chunk.Error != nil {
			fmt.Printf("\n❌ Stream error: %v\n", chunk.Error)
			break
		}
		fmt.Print(chunk.Text)
	}
	fmt.Println()

	// Example 3: Function calling with OpenRouter
	fmt.Println("\n\n3. 🔧 Function Calling")

	// Define a tool for getting weather
	weatherTool := types.NewTool(
		"get_weather",
		"Get current weather for a location",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]interface{}{
					"type":        "string",
					"description": "City and state/country",
				},
				"unit": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"celsius", "fahrenheit"},
					"description": "Temperature unit",
				},
			},
			"required": []string{"location"},
		},
	)

	messages := []types.Message{
		types.NewUserMessage("What's the weather like in Tokyo?"),
	}

	response, err := legacyClient.Text().
		Model("openai/gpt-4o-mini"). // Use model that supports function calling
		Messages(messages...).
		Tools(*weatherTool).
		MaxTokens(200).
		Generate(ctx)

	if err != nil {
		log.Printf("Function calling error: %v", err)
	} else {
		fmt.Printf("🧠 Model: openai/gpt-4o-mini\n")
		fmt.Printf("✅ Response: %s\n", response.Text)

		if len(response.ToolCalls) > 0 {
			fmt.Printf("🔧 Tool calls: %d\n", len(response.ToolCalls))
			for i, tool := range response.ToolCalls {
				fmt.Printf("   %d. %s: %s\n", i+1, tool.Function.Name, tool.Function.Arguments)
			}
		}
	}

	// Example 4: Structured output (JSON mode)
	fmt.Println("\n\n4. 📊 Structured Output (JSON)")

	jsonSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"summary": map[string]interface{}{
				"type":        "string",
				"description": "Brief summary of the concept",
			},
			"key_points": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"description": "List of key points",
			},
			"difficulty": map[string]interface{}{
				"type": "string",
				"enum": []string{"beginner", "intermediate", "advanced"},
			},
		},
		"required": []string{"summary", "key_points", "difficulty"},
	}

	structuredResponse, err := legacyClient.Structured().
		Model("openai/gpt-4o-mini").
		Prompt("Explain machine learning").
		Schema(jsonSchema).
		MaxTokens(300).
		Generate(ctx)

	if err != nil {
		log.Printf("Structured output error: %v", err)
	} else {
		fmt.Printf("🧠 Model: openai/gpt-4o-mini\n")
		fmt.Printf("✅ JSON Response: %s\n", structuredResponse.Raw)
	}

	// Example 5: Embeddings
	fmt.Println("\n\n5. 🧮 Text Embeddings")

	embeddingResponse, err := legacyClient.Embeddings().
		Model("openai/text-embedding-3-small").
		Input("The universe is vast and full of possibilities").
		Generate(ctx)

	if err != nil {
		log.Printf("Embeddings error: %v", err)
	} else {
		fmt.Printf("🧠 Model: openai/text-embedding-3-small\n")
		fmt.Printf("✅ Embedding dimensions: %d\n", len(embeddingResponse.Embeddings[0].Embedding))
		fmt.Printf("✅ First 5 values: %v\n", embeddingResponse.Embeddings[0].Embedding[:5])
	}

	// Example 6: Cost and usage tracking
	fmt.Println("\n\n6. 💰 Cost Tracking (OpenRouter)")

	// OpenRouter provides detailed usage info in response headers
	costResponse, err := legacyClient.Text().
		Model("anthropic/claude-3.5-sonnet").
		Prompt("Write a short story about parallel universes").
		MaxTokens(200).
		Generate(ctx)

	if err != nil {
		log.Printf("Cost tracking error: %v", err)
	} else {
		fmt.Printf("🧠 Model: anthropic/claude-3.5-sonnet\n")
		fmt.Printf("✅ Response length: %d characters\n", len(costResponse.Text))
		fmt.Println("💰 Check OpenRouter dashboard for detailed cost breakdown")
	}

	// Example 7: Model comparison
	fmt.Println("\n\n7. ⚖️ Model Comparison")

	prompt := "Explain the concept of emergence in complex systems"
	comparisonModels := []string{
		"openai/gpt-4o-mini",
		"anthropic/claude-3.5-sonnet",
		"google/gemini-pro",
	}

	for i, model := range comparisonModels {
		fmt.Printf("\n--- Model %d: %s ---\n", i+1, model)

		start := time.Now()
		response, err := legacyClient.Text().
			Model(model).
			Prompt(prompt).
			MaxTokens(150).
			Temperature(0.3). // Low temperature for consistent comparison
			Generate(ctx)

		duration := time.Since(start)

		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			continue
		}

		fmt.Printf("✅ Response time: %v\n", duration)
		fmt.Printf("📝 Response: %s\n", response.Text)
	}

	fmt.Println("\n🌌 OpenRouter examples complete! Check out openrouter.ai for more models and pricing.")
}
