package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

// ExampleProvider demonstrates implementing a custom provider
type ExampleProvider struct {
	config types.ProviderConfig
	client *http.Client
}

// NewExampleProvider creates a new example provider
func NewExampleProvider(config types.ProviderConfig) (types.Provider, error) {
	return &ExampleProvider{
		config: config,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (p *ExampleProvider) Name() string {
	return "example-provider"
}

func (p *ExampleProvider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	// Simple example implementation - in real usage you'd make HTTP requests to your provider
	fmt.Printf("ExampleProvider received request for model: %s\n", request.Model)

	// Get the last message content
	var prompt string
	if len(request.Messages) > 0 {
		prompt = fmt.Sprintf("%v", request.Messages[len(request.Messages)-1].GetContent())
	}
	fmt.Printf("Prompt: %s\n", prompt)

	return &types.TextResponse{
		Text:  fmt.Sprintf("Mock response from %s using model %s", p.Name(), request.Model),
		Model: request.Model,
		Metadata: map[string]interface{}{
			"provider":    p.Name(),
			"api_key":     p.config.APIKey[:8] + "...", // Don't log full key
			"base_url":    p.config.BaseURL,
			"custom_data": "This is a custom provider response",
		},
	}, nil
}

func (p *ExampleProvider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	ch := make(chan types.TextChunk)

	go func() {
		defer close(ch)

		// Simulate streaming response
		words := []string{"Hello", " from", " custom", " provider", "!"}
		for _, word := range words {
			select {
			case <-ctx.Done():
				return
			case ch <- types.TextChunk{
				Text:  word,
				Model: request.Model,
			}:
				time.Sleep(100 * time.Millisecond) // Simulate network delay
			}
		}
	}()

	return ch, nil
}

func (p *ExampleProvider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	// Simple mock structured response
	return &types.StructuredResponse{
		Data: map[string]interface{}{
			"message":  "Structured response from custom provider",
			"model":    request.Model,
			"provider": p.Name(),
		},
		Model: request.Model,
	}, nil
}

func (p *ExampleProvider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	// Mock embeddings - in real usage you'd calculate actual embeddings
	embeddings := make([]types.Embedding, len(request.Input))
	for i := range request.Input {
		// Create mock 1536-dimensional embedding
		embedding := make([]float64, 1536)
		for j := range embedding {
			embedding[j] = float64(i+j) / 1000.0 // Simple mock values
		}
		embeddings[i] = types.Embedding{
			Index:     i,
			Embedding: embedding,
		}
	}

	return &types.EmbeddingsResponse{
		Embeddings: embeddings,
		Model:      request.Model,
	}, nil
}

func (p *ExampleProvider) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	return &types.AudioResponse{
		Text:  "Mock audio transcription from custom provider",
		Model: request.Model,
	}, nil
}

func (p *ExampleProvider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	return &types.ImagesResponse{
		Images: []types.GeneratedImage{
			{
				URL: "https://example.com/mock-generated-image.png",
			},
		},
		Model: request.Model,
	}, nil
}

func main() {
	// Step 1: Configure Wormhole with your custom provider using functional options
	client := wormhole.New(
		wormhole.WithProviderConfig("example", types.ProviderConfig{
			APIKey:  "example-api-key-12345",
			BaseURL: "https://api.example.com/v1",
		}),
		wormhole.WithCustomProvider("example", NewExampleProvider),
	)

	// Step 2: Register custom models to avoid validation errors
	types.DefaultModelRegistry.Register(&types.ModelInfo{
		ID:           "example-model-v1",
		Provider:     "example",
		Capabilities: []types.ModelCapability{types.CapabilityText},
		MaxTokens:    4096,
		Description:  "Custom example model for text generation",
	})

	types.DefaultModelRegistry.Register(&types.ModelInfo{
		ID:           "example-streaming-model",
		Provider:     "example",
		Capabilities: []types.ModelCapability{types.CapabilityStream},
		MaxTokens:    4096,
		Description:  "Custom example model for streaming",
	})

	types.DefaultModelRegistry.Register(&types.ModelInfo{
		ID:           "example-structured-model",
		Provider:     "example",
		Capabilities: []types.ModelCapability{types.CapabilityStructured},
		MaxTokens:    4096,
		Description:  "Custom example model for structured output",
	})

	types.DefaultModelRegistry.Register(&types.ModelInfo{
		ID:           "example-embedding-model",
		Provider:     "example",
		Capabilities: []types.ModelCapability{types.CapabilityEmbeddings},
		MaxTokens:    4096,
		Description:  "Custom example model for embeddings",
	})

	ctx := context.Background()

	// Step 3: Use your custom provider for text generation
	fmt.Println("=== Text Generation ===")
	response, err := client.Text().
		Using("example").
		Model("example-model-v1").
		Prompt("Hello, custom provider!").
		Generate(ctx)

	if err != nil {
		log.Fatalf("Text generation failed: %v", err)
	}

	fmt.Printf("Response: %s\n", response.Text)
	fmt.Printf("Metadata: %+v\n\n", response.Metadata)

	// Step 4: Test streaming
	fmt.Println("=== Streaming ===")
	stream, err := client.Text().
		Using("example").
		Model("example-streaming-model").
		Prompt("Stream this response").
		Stream(ctx)

	if err != nil {
		log.Fatalf("Streaming failed: %v", err)
	}

	fmt.Print("Streamed response: ")
	for chunk := range stream {
		if chunk.Error != nil {
			log.Printf("Stream error: %v", chunk.Error)
			break
		}
		fmt.Print(chunk.Text)
	}
	fmt.Println()

	// Step 5: Test structured output
	fmt.Println("\n=== Structured Output ===")
	type ExampleStruct struct {
		Message string `json:"message"`
		Number  int    `json:"number"`
	}

	var result ExampleStruct
	err = client.Structured().
		Using("example").
		Model("example-structured-model").
		Prompt("Generate structured data").
		Schema(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]interface{}{"type": "string"},
				"number":  map[string]interface{}{"type": "integer"},
			},
		}).
		GenerateAs(ctx, &result)

	if err != nil {
		log.Fatalf("Structured generation failed: %v", err)
	}

	fmt.Printf("Structured response: %+v\n", result)

	// Step 6: Test embeddings
	fmt.Println("\n=== Embeddings ===")
	embeddings, err := client.Embeddings().
		Using("example").
		Model("example-embedding-model").
		Input("Hello", "World").
		Generate(ctx)

	if err != nil {
		log.Fatalf("Embeddings generation failed: %v", err)
	}

	fmt.Printf("Generated %d embeddings with %d dimensions\n",
		len(embeddings.Embeddings), len(embeddings.Embeddings[0].Embedding))

	fmt.Println("\n=== Custom Provider Registration Complete! ===")
	fmt.Println("Your custom provider is now fully integrated with Wormhole!")
}
