package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/garyblankenship/wormhole/pkg/providers/gemini"
	"github.com/garyblankenship/wormhole/pkg/providers/groq"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func main() {
	ctx := context.Background()

	// Demo Gemini provider
	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		fmt.Println("=== Testing Gemini Provider ===")
		testGemini(ctx, apiKey)
	}

	// Demo Groq provider
	if apiKey := os.Getenv("GROQ_API_KEY"); apiKey != "" {
		fmt.Println("\n=== Testing Groq Provider ===")
		testGroq(ctx, apiKey)
	}
}

func testGemini(ctx context.Context, apiKey string) {
	provider := gemini.New(apiKey, types.ProviderConfig{})

	// Test text generation
	maxTokens := 100
	request := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:     "gemini-1.5-flash",
			MaxTokens: &maxTokens,
		},
		Messages: []types.Message{
			types.NewUserMessage("What is the capital of France?"),
		},
	}

	response, err := provider.Text(ctx, request)
	if err != nil {
		log.Printf("Gemini text error: %v", err)
		return
	}

	fmt.Printf("Gemini Response: %s\n", response.Text)

	// Test embeddings
	embRequest := types.EmbeddingsRequest{
		Model: "text-embedding-004",
		Input: []string{"Hello world", "How are you?"},
	}

	embResponse, err := provider.Embeddings(ctx, embRequest)
	if err != nil {
		log.Printf("Gemini embeddings error: %v", err)
		return
	}

	fmt.Printf("Gemini Embeddings: %d vectors generated\n", len(embResponse.Embeddings))
}

func testGroq(ctx context.Context, apiKey string) {
	provider := groq.New(apiKey, types.ProviderConfig{})

	// Test text generation
	maxTokens := 100
	request := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:     "llama3-8b-8192",
			MaxTokens: &maxTokens,
		},
		Messages: []types.Message{
			types.NewUserMessage("Explain quantum computing in one sentence."),
		},
	}

	response, err := provider.Text(ctx, request)
	if err != nil {
		log.Printf("Groq text error: %v", err)
		return
	}

	fmt.Printf("Groq Response: %s\n", response.Text)

	// Test streaming
	fmt.Println("Groq Streaming:")
	stream, err := provider.Stream(ctx, request)
	if err != nil {
		log.Printf("Groq stream error: %v", err)
		return
	}

	for chunk := range stream {
		if chunk.Error != nil {
			log.Printf("Stream error: %v", chunk.Error)
			break
		}
		if chunk.Text != "" {
			fmt.Print(chunk.Text)
		}
		if chunk.FinishReason != nil {
			fmt.Printf("\n[Finished: %s]\n", *chunk.FinishReason)
			break
		}
	}
}
