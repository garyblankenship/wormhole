package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

// Example demonstrating all the user feedback improvements
func main() {
	// Configure per-provider retry settings for production reliability
	maxRetries := 3
	retryDelay := 200 * time.Millisecond
	maxRetryDelay := 10 * time.Second
	
	// Create client with per-provider retry configuration and middleware
	client := wormhole.New(
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithOpenAI("your-openai-key", types.ProviderConfig{
			MaxRetries:    &maxRetries,
			RetryDelay:    &retryDelay,
			RetryMaxDelay: &maxRetryDelay,
		}), // Custom retry settings for OpenAI
		wormhole.WithAnthropic("your-anthropic-key", types.ProviderConfig{
			MaxRetries:    &maxRetries,
			RetryDelay:    &retryDelay,
			RetryMaxDelay: &maxRetryDelay,
		}), // Same retry settings for Anthropic
		// Production-grade middleware stack
		wormhole.WithMiddleware(
			middleware.CircuitBreakerMiddleware(5, 30*time.Second), // Circuit breaking
			middleware.RateLimitMiddleware(100),                    // Rate limiting
		),
	)

	ctx := context.Background()

	// 1. Model Discovery - See what's available
	fmt.Println("=== Model Discovery ===")
	openAIModels := types.ListAvailableModels("openai")
	for _, model := range openAIModels[:3] { // Show first 3
		fmt.Printf("Model: %s\n", model.ID)
		fmt.Printf("  Description: %s\n", model.Description)
		fmt.Printf("  Context: %d tokens\n", model.ContextLength)
		if model.Cost != nil {
			fmt.Printf("  Cost: $%.4f input, $%.4f output per 1K tokens\n",
				model.Cost.InputTokens, model.Cost.OutputTokens)
		}
		fmt.Println()
	}

	// 2. Model Validation - Check capabilities
	fmt.Println("=== Model Validation ===")
	err := types.ValidateModelForCapability("gpt-5", types.CapabilityText)
	if err != nil {
		log.Printf("Validation error: %v", err)
	} else {
		fmt.Println("✅ gpt-5 supports text generation")
	}

	// Check streaming capability
	err = types.ValidateModelForCapability("gpt-5", types.CapabilityStream)
	if err != nil {
		log.Printf("Streaming not supported: %v", err)
	} else {
		fmt.Println("✅ gpt-5 supports streaming")
	}

	// 3. Cost Estimation - Know before you spend
	fmt.Println("\n=== Cost Estimation ===")
	estimatedCost, err := types.EstimateModelCost("gpt-5", 1000, 500)
	if err != nil {
		log.Printf("Cost estimation failed: %v", err)
	} else {
		fmt.Printf("Estimated cost for 1K input + 500 output tokens: $%.4f\n", estimatedCost)
	}

	// 4. Automatic Constraint Handling - GPT-5 example
	fmt.Println("\n=== Automatic Constraint Handling ===")

	// Check constraints for GPT-5
	constraints, err := types.GetModelConstraints("gpt-5")
	if err != nil {
		log.Printf("Failed to get constraints: %v", err)
	} else {
		fmt.Printf("GPT-5 constraints: %+v\n", constraints)
	}

	// This will automatically set temperature=1.0 for GPT-5
	response, err := client.Text().
		Model("gpt-5").
		Prompt("Tell me about automatic constraint handling").
		Generate(ctx)

	if err != nil {
		// 5. Typed Error Handling - Know exactly what went wrong
		if wormholeErr, ok := types.AsWormholeError(err); ok {
			fmt.Printf("Typed error occurred:\n")
			fmt.Printf("  Code: %s\n", wormholeErr.Code)
			fmt.Printf("  Message: %s\n", wormholeErr.Message)
			fmt.Printf("  Retryable: %t\n", wormholeErr.Retryable)
			fmt.Printf("  Details: %s\n", wormholeErr.Details)
			if wormholeErr.Model != "" {
				fmt.Printf("  Model: %s\n", wormholeErr.Model)
			}
		} else {
			log.Printf("Generic error: %v", err)
		}
	} else {
		fmt.Printf("Response: %s\n", response.Text[:100]+"...")
	}

	// 6. Streaming with Error Handling
	fmt.Println("\n=== Streaming Response ===")
	stream, err := client.Text().
		Model("gpt-5-mini"). // Use a model we know works
		Prompt("Count from 1 to 5, one number per response").
		Stream(ctx)

	if err != nil {
		log.Printf("Streaming failed: %v", err)
		return
	}

	fmt.Print("Streaming: ")
	for chunk := range stream {
		// Check for errors in chunks
		if chunk.Error != nil {
			log.Printf("Stream error: %v", chunk.Error)
			break
		}

		// Print the content
		if chunk.Text != "" {
			fmt.Print(chunk.Text)
		}

		// Check for completion
		if chunk.FinishReason != nil {
			fmt.Printf("\n[Finished: %s]\n", *chunk.FinishReason)
			break
		}
	}

	// 7. Provider Error Handling (No Automatic Fallback)
	fmt.Println("\n=== Provider Error Handling ===")

	// Attempt with specific provider and handle errors explicitly
	primaryResponse, err := attemptWithErrorHandling(client, ctx)
	if err != nil {
		log.Printf("Provider request failed: %v", err)
	} else {
		fmt.Printf("Primary response: %s\n", primaryResponse.Text[:100]+"...")
	}

	// 8. Request Validation
	fmt.Println("\n=== Request Validation ===")

	// This will fail validation - no model specified
	_, err = client.Text().
		Prompt("This should fail").
		Generate(ctx)

	if err != nil {
		fmt.Printf("Expected validation error: %v\n", err)
	}

	fmt.Println("\n=== All Examples Complete ===")
}

// attemptWithErrorHandling demonstrates error handling without automatic fallback
func attemptWithErrorHandling(client *wormhole.Wormhole, ctx context.Context) (*types.TextResponse, error) {
	// Attempt with specific provider
	response, err := client.Text().
		Using("openai").
		Model("gpt-5-mini").
		Prompt("Hello from provider").
		Generate(ctx)

	if err != nil {
		// Check if it's a typed error for better error handling
		if wormholeErr, ok := types.AsWormholeError(err); ok {
			switch wormholeErr.Code {
			case types.ErrorCodeRateLimit:
				return nil, fmt.Errorf("rate limit exceeded: %w", wormholeErr)

			case types.ErrorCodeTimeout:
				return nil, fmt.Errorf("request timed out: %w", wormholeErr)

			case types.ErrorCodeNetwork:
				return nil, fmt.Errorf("network error: %w", wormholeErr)

			case types.ErrorCodeAuth:
				return nil, fmt.Errorf("authentication failed: %w", wormholeErr)

			default:
				return nil, fmt.Errorf("provider error (%s): %w", wormholeErr.Code, wormholeErr)
			}
		}
		return nil, fmt.Errorf("unexpected error: %w", err)
	}

	return response, nil
}
