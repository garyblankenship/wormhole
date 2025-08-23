package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("MISTRAL_API_KEY")
	if apiKey == "" {
		log.Fatal("MISTRAL_API_KEY environment variable is required")
	}

	// Create wormhole client with Mistral provider
	client := wormhole.New(
		wormhole.WithDefaultProvider("mistral"),
		wormhole.WithMistral(types.ProviderConfig{
			APIKey: apiKey,
		}),
	)

	// Example 1: Simple text generation
	fmt.Println("=== Text Generation ===")

	response, err := client.Text().
		Model("mistral-large-latest").
		Messages(types.NewUserMessage("What is the capital of France?")).
		Generate(context.Background())
	if err != nil {
		log.Printf("Text generation error: %v", err)
	} else {
		fmt.Printf("Response: %s\n", response.Text)
		fmt.Printf("Model: %s\n", response.Model)
		fmt.Printf("Usage: %+v\n", response.Usage)
	}

	fmt.Println()

	// Example 2: Embeddings
	fmt.Println("=== Embeddings ===")

	embeddingsResponse, err := client.Embeddings().
		Model("mistral-embed").
		Input("Hello, world!", "How are you?").
		Generate(context.Background())
	if err != nil {
		log.Printf("Embeddings generation error: %v", err)
	} else {
		fmt.Printf("Generated %d embeddings\n", len(embeddingsResponse.Embeddings))
		for i, embedding := range embeddingsResponse.Embeddings {
			fmt.Printf("Embedding %d: %d dimensions\n", i, len(embedding.Embedding))
		}
		fmt.Printf("Model: %s\n", embeddingsResponse.Model)
		fmt.Printf("Usage: %+v\n", embeddingsResponse.Usage)
	}

	fmt.Println()

	// Example 3: Streaming
	fmt.Println("=== Streaming Text Generation ===")

	stream, err := client.Text().
		Model("mistral-large-latest").
		Messages(types.NewUserMessage("Tell me a short story about a brave knight.")).
		Stream(context.Background())
	if err != nil {
		log.Printf("Streaming error: %v", err)
	} else {
		fmt.Print("Streaming response: ")
		for chunk := range stream {
			if chunk.Error != nil {
				log.Printf("Stream error: %v", chunk.Error)
				break
			}
			if chunk.Delta != nil && chunk.Delta.Content != "" {
				fmt.Print(chunk.Delta.Content)
			}
			if chunk.FinishReason != nil {
				fmt.Printf("\nFinish reason: %s\n", *chunk.FinishReason)
				break
			}
		}
	}

	fmt.Println()

	// Example 4: Structured output with function calling
	fmt.Println("=== Structured Output ===")

	var result map[string]any
	err = client.Structured().
		Model("mistral-large-latest").
		Messages(types.NewUserMessage("Extract the name, age, and occupation from: 'John Doe is a 30-year-old software engineer.'")).
		Schema(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "The person's full name",
				},
				"age": map[string]any{
					"type":        "integer",
					"description": "The person's age",
				},
				"occupation": map[string]any{
					"type":        "string",
					"description": "The person's job or occupation",
				},
			},
			"required": []string{"name", "age", "occupation"},
		}).
		GenerateAs(context.Background(), &result)
	if err != nil {
		log.Printf("Structured generation error: %v", err)
	} else {
		fmt.Printf("Structured data: %+v\n", result)
	}
}
