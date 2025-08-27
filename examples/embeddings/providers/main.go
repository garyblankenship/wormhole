package main

import (
	"context"
	"fmt"
	"os"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	// Create a new Wormhole client
	client := wormhole.New(wormhole.WithOpenAI("your-api-key"))

	fmt.Println("=== OpenAI-Compatible Providers for Embeddings ===")
	fmt.Println("Groq, Mistral, and many other providers use OpenAI-compatible APIs")
	fmt.Println("This means NO separate provider implementation needed!")
	fmt.Println()

	ctx := context.Background()

	// Example 1: Groq embeddings (if they support embeddings)
	fmt.Println("--- Groq Example ---")
	if os.Getenv("GROQ_API_KEY") != "" {
		// Note: As of 2024, Groq focuses on text generation, not embeddings
		// But if they add embeddings support, this is how you'd use it:
		fmt.Println("Groq primarily focuses on fast text generation, not embeddings")
		fmt.Println("If embeddings become available, usage would be:")
		fmt.Println(`
response, err := client.Embeddings().
    BaseURL("https://api.groq.com/openai/v1").
    Model("groq-embedding-model").  // Hypothetical model
    Input("Text to embed").
    Generate(ctx)`)
	} else {
		fmt.Println("GROQ_API_KEY not set - skipping Groq example")
	}

	// Example 2: Mistral embeddings
	fmt.Println("\n--- Mistral Example ---")
	if os.Getenv("MISTRAL_API_KEY") != "" {
		// Use Mistral's embedding model via BaseURL
		response, err := client.Embeddings().
			BaseURL("https://api.mistral.ai/v1").
			Model("mistral-embed"). // Mistral's embedding model
			Input("Mistral embeddings via OpenAI-compatible API").
			Generate(ctx)

		if err != nil {
			fmt.Printf("Mistral embeddings failed: %v\n", err)
		} else {
			fmt.Printf("✓ Generated Mistral embedding: %d dimensions\n",
				len(response.Embeddings[0].Embedding))
		}
	} else {
		fmt.Println("MISTRAL_API_KEY not set - skipping Mistral example")
		fmt.Println("If available, usage would be:")
		fmt.Println(`
client := wormhole.New(wormhole.WithOpenAI(os.Getenv("MISTRAL_API_KEY")))

response, err := client.Embeddings().
    BaseURL("https://api.mistral.ai/v1").
    Model("mistral-embed").
    Input("Text to embed with Mistral").
    Generate(ctx)`)
	}

	// Example 3: Any other OpenAI-compatible provider
	fmt.Println("\n--- Generic OpenAI-Compatible Provider ---")
	fmt.Println("The pattern works for ANY provider that implements OpenAI's API:")
	fmt.Println(`
// Step 1: Initialize with your API key
client := wormhole.New(wormhole.WithOpenAI("your-provider-api-key"))

// Step 2: Use BaseURL to point to their API
response, err := client.Embeddings().
    BaseURL("https://api.your-provider.com/v1").  // Provider's endpoint
    Model("provider-embedding-model").            // Provider's model name
    Input("Text to embed").
    Generate(ctx)`)

	// Example 4: Popular OpenAI-compatible services
	fmt.Println("\n--- Popular OpenAI-Compatible Services ---")
	popularServices := map[string]string{
		"OpenRouter":      "https://openrouter.ai/api/v1",
		"Together.ai":     "https://api.together.xyz/v1",
		"Perplexity":      "https://api.perplexity.ai",
		"Replicate":       "https://api.replicate.com/v1",
		"Hugging Face":    "https://api-inference.huggingface.co/v1",
		"Local LM Studio": "http://localhost:1234/v1",
		"Local Ollama":    "http://localhost:11434/v1",
	}

	fmt.Println("Services that work with the BaseURL approach:")
	for service, endpoint := range popularServices {
		fmt.Printf("- %-15s: %s\n", service, endpoint)
	}

	fmt.Println("\n=== Why This Architecture is Brilliant ===")
	fmt.Println("✅ Zero code duplication - one provider handles all OpenAI-compatible APIs")
	fmt.Println("✅ No waiting for Wormhole updates - use any new provider immediately")
	fmt.Println("✅ Consistent API - same methods work across all providers")
	fmt.Println("✅ Automatic middleware - all Wormhole features work with any provider")
	fmt.Println("✅ Easy switching - change just the BaseURL to try different providers")

	fmt.Println("\n=== Production Tips ===")
	fmt.Println("1. Check provider documentation for their specific model names")
	fmt.Println("2. Some providers may have slight API differences - test thoroughly")
	fmt.Println("3. Rate limits and pricing vary by provider")
	fmt.Println("4. Use environment variables for API keys and endpoints")
	fmt.Println("5. Implement fallback logic to switch providers if one fails")

	// Example 5: Provider fallback pattern
	fmt.Println("\n--- Provider Fallback Pattern ---")
	fmt.Println("For production, implement provider fallback logic:")
	fmt.Println("- Try multiple endpoints when primary fails")
	fmt.Println("- Use different models based on availability")
	fmt.Println("- Implement retry logic with exponential backoff")
}
