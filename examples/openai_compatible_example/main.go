package main

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v2"
)

func main() {
	fmt.Println("=== OpenAI-Compatible APIs with BaseURL ===")
	fmt.Println("🚀 NEW: Zero configuration needed - just change the BaseURL!")

	// Simple setup - one client can access ANY OpenAI-compatible API
	client := wormhole.New(
		wormhole.WithOpenAI("your-api-key-if-needed"), // Default config
	)

	ctx := context.Background()

	// Example 1: LMStudio (local) - just change BaseURL!
	fmt.Println("--- Example 1: LMStudio (local) ---")
	_, err := client.Text().
		BaseURL("http://localhost:1234/v1"). // ✨ That's it!
		Model("llama-3.2-8b").
		Prompt("Write a short story about time travel").
		Temperature(0.9).
		MaxTokens(150).
		Generate(ctx)

	if err != nil {
		fmt.Printf("LMStudio: %v (expected if not running locally)\n", err)
	} else {
		fmt.Println("✅ LMStudio: Success!")
	}

	// Example 2: vLLM (local/remote) - just change BaseURL!
	fmt.Println("--- Example 2: vLLM (local/remote) ---")
	_, err = client.Text().
		BaseURL("http://localhost:8000/v1"). // ✨ That's it!
		Model("codellama-13b").
		Prompt("Write a Python function to calculate fibonacci numbers").
		Temperature(0.2).
		MaxTokens(200).
		Generate(ctx)

	if err != nil {
		fmt.Printf("vLLM: %v (expected if not running locally)\n", err)
	} else {
		fmt.Println("✅ vLLM: Success!")
	}

	// Example 3: Ollama (local) - just change BaseURL!
	fmt.Println("--- Example 3: Ollama (local) ---")
	_, err = client.Text().
		BaseURL("http://localhost:11434/v1"). // ✨ That's it!
		Model("llama3.2").
		Prompt("Explain quantum computing in simple terms").
		Temperature(0.5).
		MaxTokens(100).
		Generate(ctx)

	if err != nil {
		fmt.Printf("Ollama: %v (expected if not running locally)\n", err)
	} else {
		fmt.Println("✅ Ollama: Success!")
	}

	// Example 4: OpenRouter - just change BaseURL!
	fmt.Println("--- Example 4: OpenRouter (cloud) ---")
	_, err = client.Text().
		BaseURL("https://openrouter.ai/api/v1"). // ✨ That's it!
		Model("anthropic/claude-sonnet-4-5").
		Prompt("Hello from OpenRouter!").
		MaxTokens(50).
		Generate(ctx)

	if err != nil {
		fmt.Printf("OpenRouter: %v (expected without API key)\n", err)
	} else {
		fmt.Println("✅ OpenRouter: Success!")
	}

	// Example 5: Any custom OpenAI-compatible API
	fmt.Println("--- Example 5: Custom API ---")
	_, err = client.Text().
		BaseURL("https://api.your-custom-service.com/v1"). // ✨ Just change URL!
		Model("your-custom-model").
		Prompt("Hello from custom API!").
		MaxTokens(50).
		Generate(ctx)

	if err != nil {
		fmt.Printf("Custom API: %v (expected - not a real endpoint)\n", err)
	} else {
		fmt.Println("✅ Custom API: Success!")
	}

	fmt.Println("\n🎉 NEW ARCHITECTURE BENEFITS:")
	fmt.Println("✅ Zero configuration - just change BaseURL")
	fmt.Println("✅ No more separate provider packages")
	fmt.Println("✅ Works with ANY OpenAI-compatible API")
	fmt.Println("✅ Consistent API across all providers")
	fmt.Println("✅ Simple and maintainable")

	fmt.Println("\n📋 SUPPORTED APIs:")
	fmt.Println("• LMStudio: http://localhost:1234/v1")
	fmt.Println("• vLLM: http://localhost:8000/v1")
	fmt.Println("• Ollama: http://localhost:11434/v1")
	fmt.Println("• OpenRouter: https://openrouter.ai/api/v1")
	fmt.Println("• Any custom OpenAI-compatible API")
}
