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
	// Create Wormhole client with Gemini provider
	client := wormhole.New(
		wormhole.WithGemini(apiKey, types.ProviderConfig{}),
	)

	// Test text generation
	maxTokens := 100
	response, err := client.Text().
		Using("gemini").
		Model("gemini-1.5-flash").
		MaxTokens(maxTokens).
		Messages(types.NewUserMessage("What is the capital of France?")).
		Generate(ctx)

	if err != nil {
		log.Printf("Gemini text error: %v", err)
		return
	}

	fmt.Printf("Gemini Response: %s\n", response.Text)

	// Test embeddings
	embResponse, err := client.Embeddings().
		Using("gemini").
		Model("text-embedding-004").
		Input("Hello world", "How are you?").
		Generate(ctx)

	if err != nil {
		log.Printf("Gemini embeddings error: %v", err)
		return
	}

	fmt.Printf("Gemini Embeddings: %d vectors generated\n", len(embResponse.Embeddings))
}

func testGroq(ctx context.Context, apiKey string) {
	// Create Wormhole client with Groq provider (now using OpenAI-compatible)
	client := wormhole.New(
		wormhole.WithGroq(apiKey),
	)

	// Test text generation
	maxTokens := 100
	response, err := client.Text().
		Using("groq").
		Model("llama3-8b-8192").
		MaxTokens(maxTokens).
		Messages(types.NewUserMessage("Explain quantum computing in one sentence.")).
		Generate(ctx)

	if err != nil {
		log.Printf("Groq text error: %v", err)
		return
	}

	fmt.Printf("Groq Response: %s\n", response.Text)

	// Test streaming
	fmt.Println("Groq Streaming:")
	stream, err := client.Text().
		Using("groq").
		Model("llama3-8b-8192").
		MaxTokens(maxTokens).
		Messages(types.NewUserMessage("Explain quantum computing in one sentence.")).
		Stream(ctx)

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
