package main

import (
	"context"
	"fmt"
	"log"

	"github.com/garyblankenship/wormhole/pkg/providers/ollama"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func main() {
	// Create Ollama provider (assumes Ollama is running locally on default port)
	config := types.ProviderConfig{
		BaseURL: "http://localhost:11434", // Default Ollama URL
	}

	provider, err := ollama.New(config)
	if err != nil {
		log.Fatalf("Failed to create Ollama provider: %v", err)
	}

	// Example 1: Simple text generation
	fmt.Println("=== Text Generation Example ===")
	textRequest := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "llama2", // or any model you have installed
		},
		Messages: []types.Message{
			&types.UserMessage{Content: "What is the capital of France?"},
		},
	}

	response, err := provider.Text(context.Background(), textRequest)
	if err != nil {
		log.Printf("Text generation error: %v", err)
	} else {
		fmt.Printf("Response: %s\n", response.Text)
		fmt.Printf("Model: %s\n", response.Model)
		if response.Usage != nil {
			fmt.Printf("Tokens used: %d\n", response.Usage.TotalTokens)
		}
	}

	// Example 2: Streaming text generation
	fmt.Println("\n=== Streaming Example ===")
	streamRequest := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "llama2",
		},
		Messages: []types.Message{
			&types.UserMessage{Content: "Write a short poem about programming."},
		},
	}

	chunks, err := provider.Stream(context.Background(), streamRequest)
	if err != nil {
		log.Printf("Streaming error: %v", err)
	} else {
		fmt.Print("Streaming response: ")
		for chunk := range chunks {
			if chunk.Delta != nil && chunk.Delta.Content != "" {
				fmt.Print(chunk.Delta.Content)
			}
			if chunk.FinishReason != nil {
				fmt.Printf("\nFinish reason: %s\n", *chunk.FinishReason)
			}
		}
	}

	// Example 3: Structured output (JSON mode)
	fmt.Println("\n=== Structured Output Example ===")
	structuredRequest := types.StructuredRequest{
		BaseRequest: types.BaseRequest{
			Model: "llama2",
		},
		Messages: []types.Message{
			&types.UserMessage{Content: "Describe Paris in JSON format with name, country, population, and famous_landmarks fields."},
		},
		Mode: types.StructuredModeJSON,
	}

	structuredResponse, err := provider.Structured(context.Background(), structuredRequest)
	if err != nil {
		log.Printf("Structured output error: %v", err)
	} else {
		fmt.Printf("Structured data: %+v\n", structuredResponse.Data)
	}

	// Example 4: Embeddings
	fmt.Println("\n=== Embeddings Example ===")
	embeddingsRequest := types.EmbeddingsRequest{
		Model: "llama2", // Note: Not all models support embeddings
		Input: []string{"Hello, world!", "This is a test sentence."},
	}

	embeddingsResponse, err := provider.Embeddings(context.Background(), embeddingsRequest)
	if err != nil {
		log.Printf("Embeddings error: %v", err)
	} else {
		fmt.Printf("Generated %d embeddings\n", len(embeddingsResponse.Embeddings))
		if len(embeddingsResponse.Embeddings) > 0 {
			fmt.Printf("First embedding dimensions: %d\n", len(embeddingsResponse.Embeddings[0].Embedding))
		}
	}

	// Example 5: List available models
	fmt.Println("\n=== Available Models ===")
	models, err := provider.ListModels(context.Background())
	if err != nil {
		log.Printf("List models error: %v", err)
	} else {
		fmt.Printf("Available models:\n")
		for _, model := range models.Models {
			fmt.Printf("- %s (size: %d bytes)\n", model.Name, model.Size)
		}
	}
}
