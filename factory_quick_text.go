package wormhole

import (
	"context"

	"github.com/garyblankenship/wormhole/v2/types"
)

// ==================== Ultra-Quick One-Liners ====================
// These functions provide the absolute minimum path from idea to working code.

// QuickText generates text with minimal configuration.
// This is the fastest path to a working LLM call - perfect for scripts, demos, and prototyping.
//
// Example:
//
//	response, err := wormhole.QuickText("gpt-4o", "What is Go?", os.Getenv("OPENAI_API_KEY"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(response.Text)
func QuickText(model, prompt, apiKey string) (*types.TextResponse, error) {
	return QuickTextWithContext(context.Background(), model, prompt, apiKey)
}

// QuickTextWithContext generates text with context support for cancellation/timeout.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	response, err := wormhole.QuickTextWithContext(ctx, "gpt-4o", "What is Go?", apiKey)
func QuickTextWithContext(ctx context.Context, model, prompt, apiKey string) (*types.TextResponse, error) {
	return QuickOpenAI(apiKey).Text().
		Model(model).
		Prompt(prompt).
		Generate(ctx)
}

// QuickChat generates a response in a conversation with system context.
// This is useful for chat-like interactions where you need a system prompt.
//
// Example:
//
//	response, err := wormhole.QuickChat(
//	    "gpt-4o",
//	    "You are a helpful coding assistant.",
//	    "How do I read a file in Go?",
//	    os.Getenv("OPENAI_API_KEY"),
//	)
func QuickChat(model, systemPrompt, userMessage, apiKey string) (*types.TextResponse, error) {
	return QuickChatWithContext(context.Background(), model, systemPrompt, userMessage, apiKey)
}

// QuickChatWithContext generates a chat response with context support.
func QuickChatWithContext(ctx context.Context, model, systemPrompt, userMessage, apiKey string) (*types.TextResponse, error) {
	return QuickOpenAI(apiKey).Text().
		Model(model).
		SystemPrompt(systemPrompt).
		Prompt(userMessage).
		Generate(ctx)
}

// QuickStream streams text generation for real-time output.
//
// Example:
//
//	stream, err := wormhole.QuickStream("gpt-4o", "Write a haiku", apiKey)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for chunk := range stream {
//	    fmt.Print(chunk.Text)
//	}
func QuickStream(model, prompt, apiKey string) (<-chan types.TextChunk, error) {
	return QuickStreamWithContext(context.Background(), model, prompt, apiKey)
}

// QuickStreamWithContext streams text with context support for cancellation.
func QuickStreamWithContext(ctx context.Context, model, prompt, apiKey string) (<-chan types.TextChunk, error) {
	return QuickOpenAI(apiKey).Text().
		Model(model).
		Prompt(prompt).
		Stream(ctx)
}
