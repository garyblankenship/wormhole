package main

import (
	"context"
	"fmt"
	"log"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	// Create a new Wormhole client with multiple OpenAI-compatible providers
	fmt.Println("=== Setting up Multiple OpenAI-Compatible Providers ===")
	p := wormhole.New(
		// Example 1: LMStudio (local)
		wormhole.WithLMStudio(types.ProviderConfig{
			BaseURL: "http://localhost:1234/v1", // Default LMStudio port
			Timeout: 30,
		}),
		// Example 2: vLLM (local or remote)
		wormhole.WithVLLM(types.ProviderConfig{
			BaseURL: "http://localhost:8000/v1", // Default vLLM port
			Timeout: 60,
		}),
		// Example 3: Ollama OpenAI-compatible API
		wormhole.WithOllamaOpenAI(types.ProviderConfig{
			BaseURL: "http://localhost:11434/v1", // Ollama OpenAI-compatible endpoint
			Timeout: 30,
		}),
		// Example 4: Generic OpenAI-compatible provider (e.g., hosted service)
		wormhole.WithOpenAICompatible("my-custom-llm", "https://api.my-llm-service.com/v1", types.ProviderConfig{
			APIKey:  "your-api-key-if-needed",
			Timeout: 30,
			Headers: map[string]string{
				"X-Custom-Header": "custom-value",
			},
		}),
	)

	// Example 5: Multiple providers with different use cases
	fmt.Println("\n=== Using Different Providers for Different Tasks ===")

	// Use LMStudio for creative writing
	fmt.Println("--- Creative Writing with LMStudio ---")
	creativeResponse, err := p.Text().
		Using("lmstudio").
		Model("creative-model"). // Whatever model you have loaded
		Prompt("Write a creative story about time travel").
		Temperature(0.9).
		MaxTokens(150).
		Generate(context.Background())

	if err != nil {
		log.Printf("LMStudio error: %v", err)
	} else {
		fmt.Printf("Creative story: %s\n\n", creativeResponse.Text)
	}

	// Use vLLM for code generation
	fmt.Println("--- Code Generation with vLLM ---")
	codeResponse, err := p.Text().
		Using("vllm").
		Model("code-model").
		Prompt("Write a Python function to calculate fibonacci numbers").
		Temperature(0.2). // Lower temperature for more deterministic code
		MaxTokens(200).
		Generate(context.Background())

	if err != nil {
		log.Printf("vLLM error: %v", err)
	} else {
		fmt.Printf("Generated code: %s\n\n", codeResponse.Text)
	}

	// Use Ollama OpenAI API for structured data
	fmt.Println("--- Structured Data with Ollama OpenAI API ---")
	type Product struct {
		Name        string  `json:"name"`
		Price       float64 `json:"price"`
		Category    string  `json:"category"`
		Description string  `json:"description"`
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Product name",
			},
			"price": map[string]interface{}{
				"type":        "number",
				"description": "Product price in USD",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Product category",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Product description",
			},
		},
		"required": []string{"name", "price", "category"},
	}

	var product Product
	err = p.Structured().
		Using("ollama-openai").
		Model("llama2"). // Or whatever model you have in Ollama
		Prompt("Generate a product for an electronics store").
		Schema(schema).
		GenerateAs(context.Background(), &product)

	if err != nil {
		log.Printf("Ollama structured generation error: %v", err)
	} else {
		fmt.Printf("Generated product: %+v\n\n", product)
	}

	// Example 6: Function calling with tools
	fmt.Println("--- Function Calling Example ---")
	calculatorTool := types.NewTool(
		"calculate",
		"Perform basic arithmetic operations",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"operation": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"add", "subtract", "multiply", "divide"},
					"description": "The arithmetic operation to perform",
				},
				"a": map[string]interface{}{
					"type":        "number",
					"description": "First number",
				},
				"b": map[string]interface{}{
					"type":        "number",
					"description": "Second number",
				},
			},
			"required": []string{"operation", "a", "b"},
		},
	)

	toolResponse, err := p.Text().
		Using("lmstudio"). // Use whichever provider supports function calling
		Model("function-calling-model").
		Prompt("What is 15 multiplied by 7?").
		Tools(*calculatorTool).
		Generate(context.Background())

	if err != nil {
		log.Printf("Function calling error: %v", err)
	} else {
		fmt.Printf("Response: %s\n", toolResponse.Text)
		for _, toolCall := range toolResponse.ToolCalls {
			fmt.Printf("Function called: %s with args: %+v\n", toolCall.Name, toolCall.Arguments)
		}
	}

	// Example 7: Streaming with different providers
	fmt.Println("\n--- Streaming Example ---")
	stream, err := p.Text().
		Using("lmstudio").
		Model("streaming-model").
		Prompt("Explain quantum computing in simple terms").
		Temperature(0.5).
		Stream(context.Background())

	if err != nil {
		log.Printf("Streaming error: %v", err)
	} else {
		fmt.Print("Streaming explanation: ")
		for chunk := range stream {
			if chunk.Error != nil {
				log.Printf("Stream error: %v", chunk.Error)
				break
			}
			fmt.Print(chunk.Text)
		}
		fmt.Println()
	}

	fmt.Println("\n=== OpenAI-Compatible Providers Example Complete ===")
	fmt.Println("Supported providers: LMStudio, vLLM, Ollama OpenAI API, and any other OpenAI-compatible service")
}
