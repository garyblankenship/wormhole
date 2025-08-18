package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/garyblankenship/wormhole/pkg/providers/mistral"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("MISTRAL_API_KEY")
	if apiKey == "" {
		log.Fatal("MISTRAL_API_KEY environment variable is required")
	}

	// Create Mistral provider
	config := types.ProviderConfig{
		APIKey: apiKey,
	}

	provider := mistral.New(config)

	// Example 1: Simple text generation
	fmt.Println("=== Text Generation ===")
	textRequest := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "mistral-large-latest",
		},
		Messages: []types.Message{
			&types.UserMessage{
				Content: "What is the capital of France?",
			},
		},
	}

	response, err := provider.Text(context.Background(), textRequest)
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
	embeddingsRequest := types.EmbeddingsRequest{
		Model: "mistral-embed",
		Input: []string{
			"Hello, world!",
			"How are you?",
		},
	}

	embeddingsResponse, err := provider.Embeddings(context.Background(), embeddingsRequest)
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
	streamRequest := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "mistral-large-latest",
		},
		Messages: []types.Message{
			&types.UserMessage{
				Content: "Tell me a short story about a brave knight.",
			},
		},
	}

	stream, err := provider.Stream(context.Background(), streamRequest)
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
	structuredRequest := types.StructuredRequest{
		BaseRequest: types.BaseRequest{
			Model: "mistral-large-latest",
		},
		Messages: []types.Message{
			&types.UserMessage{
				Content: "Extract the name, age, and occupation from: 'John Doe is a 30-year-old software engineer.'",
			},
		},
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "The person's full name",
				},
				"age": map[string]interface{}{
					"type":        "integer",
					"description": "The person's age",
				},
				"occupation": map[string]interface{}{
					"type":        "string",
					"description": "The person's job or occupation",
				},
			},
			"required": []string{"name", "age", "occupation"},
		},
		SchemaName: "person_info",
		Mode:       types.StructuredModeTools,
	}

	structuredResponse, err := provider.Structured(context.Background(), structuredRequest)
	if err != nil {
		log.Printf("Structured generation error: %v", err)
	} else {
		fmt.Printf("Structured data: %+v\n", structuredResponse.Data)
		fmt.Printf("Model: %s\n", structuredResponse.Model)
		fmt.Printf("Usage: %+v\n", structuredResponse.Usage)
	}
}
