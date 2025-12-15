package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	// Create a new Wormhole client
	client := wormhole.New()

	// Ensure we have an OpenAI API key for this example
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Basic embedding generation with OpenAI
	fmt.Println("=== Basic Embeddings Example ===")

	response, err := client.Embeddings().
		Using("openai").
		Model("text-embedding-3-small").
		Input("Hello, world!", "This is a test embedding").
		Dimensions(512). // Optional: specify dimensions for 3-small/3-large models
		Generate(context.Background())

	if err != nil {
		log.Fatalf("Failed to generate embeddings: %v", err)
	}

	fmt.Printf("Generated %d embeddings:\n", len(response.Embeddings))
	for i, embedding := range response.Embeddings {
		fmt.Printf("Embedding %d: [%.6f, %.6f, %.6f, ...] (%d dimensions)\n",
			i,
			embedding.Embedding[0],
			embedding.Embedding[1],
			embedding.Embedding[2],
			len(embedding.Embedding))
	}

	// Using Ollama (requires local Ollama installation)
	fmt.Println("\n=== Ollama Embeddings Example ===")

	ollamaResponse, err := client.Embeddings().
		Using("ollama").
		Model("nomic-embed-text"). // Popular embedding model for Ollama
		Input("Local embedding generation", "Works offline").
		Generate(context.Background())

	if err != nil {
		fmt.Printf("Ollama embeddings failed (may not be installed): %v\n", err)
	} else {
		fmt.Printf("Generated %d local embeddings:\n", len(ollamaResponse.Embeddings))
		for i, embedding := range ollamaResponse.Embeddings {
			fmt.Printf("Local Embedding %d: %d dimensions\n", i, len(embedding.Embedding))
		}
	}

	// Using Gemini
	fmt.Println("\n=== Gemini Embeddings Example ===")

	if os.Getenv("GEMINI_API_KEY") != "" {
		geminiResponse, err := client.Embeddings().
			Using("gemini").
			Model("models/embedding-001"). // Gemini embedding model
			Input("Gemini embeddings", "Google's embedding service").
			Generate(context.Background())

		if err != nil {
			fmt.Printf("Gemini embeddings failed: %v\n", err)
		} else {
			fmt.Printf("Generated %d Gemini embeddings:\n", len(geminiResponse.Embeddings))
			for i, embedding := range geminiResponse.Embeddings {
				fmt.Printf("Gemini Embedding %d: %d dimensions\n", i, len(embedding.Embedding))
			}
		}
	} else {
		fmt.Println("Skipping Gemini example (GEMINI_API_KEY not set)")
	}

	// Demonstrate error handling for unsupported providers
	fmt.Println("\n=== Error Handling Example ===")

	_, err = client.Embeddings().
		Using("anthropic"). // Anthropic doesn't support embeddings
		Model("any-model").
		Input("This will fail").
		Generate(context.Background())

	if err != nil {
		fmt.Printf("Expected error for unsupported provider: %v\n", err)
	}

	// Show usage statistics if available
	if response.Usage != nil {
		fmt.Printf("\nUsage Statistics:\n")
		fmt.Printf("- Prompt Tokens: %d\n", response.Usage.PromptTokens)
		fmt.Printf("- Total Tokens: %d\n", response.Usage.TotalTokens)
	}
}
