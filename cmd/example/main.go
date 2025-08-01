package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/prism-php/prism-go/pkg/prism"
	"github.com/prism-php/prism-go/pkg/types"
)

func main() {
	// Initialize Prism with providers
	p := prism.New(prism.Config{
		DefaultProvider: "openai",
		Providers: map[string]types.ProviderConfig{
			"openai": {
				APIKey: os.Getenv("OPENAI_API_KEY"),
			},
			"anthropic": {
				APIKey: os.Getenv("ANTHROPIC_API_KEY"),
			},
		},
	})

	ctx := context.Background()

	// Example 1: Simple text generation
	fmt.Println("=== Example 1: Simple Text Generation ===")
	simpleExample(ctx, p)

	// Example 2: Conversation with messages
	fmt.Println("\n=== Example 2: Conversation ===")
	conversationExample(ctx, p)

	// Example 3: Streaming response
	fmt.Println("\n=== Example 3: Streaming ===")
	streamingExample(ctx, p)

	// Example 4: Structured output
	fmt.Println("\n=== Example 4: Structured Output ===")
	structuredExample(ctx, p)

	// Example 5: Tool calling
	fmt.Println("\n=== Example 5: Tool Calling ===")
	toolExample(ctx, p)
}

func simpleExample(ctx context.Context, p *prism.Prism) {
	response, err := p.Text().
		Model("gpt-3.5-turbo").
		Prompt("Write a haiku about Go programming").
		Temperature(0.7).
		Generate(ctx)

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Haiku:")
	fmt.Println(response.Text)
	fmt.Printf("Tokens used: %d\n", response.Usage.TotalTokens)
}

func conversationExample(ctx context.Context, p *prism.Prism) {
	messages := []types.Message{
		types.NewSystemMessage("You are a helpful assistant who speaks like a pirate"),
		types.NewUserMessage("What is the capital of France?"),
		types.NewAssistantMessage("Arrr, the capital of France be Paris, me hearty!"),
		types.NewUserMessage("What's the population?"),
	}

	response, err := p.Text().
		Model("gpt-3.5-turbo").
		Messages(messages...).
		Generate(ctx)

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Assistant:", response.Text)
}

func streamingExample(ctx context.Context, p *prism.Prism) {
	chunks, err := p.Text().
		Model("gpt-3.5-turbo").
		Prompt("Count from 1 to 5 slowly").
		Stream(ctx)

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Print("Streaming: ")
	for chunk := range chunks {
		if chunk.Error != nil {
			log.Printf("\nStream error: %v\n", chunk.Error)
			break
		}
		fmt.Print(chunk.Delta)
	}
	fmt.Println()
}

func structuredExample(ctx context.Context, p *prism.Prism) {
	type ProductInfo struct {
		Name     string  `json:"name"`
		Price    float64 `json:"price"`
		InStock  bool    `json:"in_stock"`
		Category string  `json:"category"`
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name":     map[string]string{"type": "string"},
			"price":    map[string]string{"type": "number"},
			"in_stock": map[string]string{"type": "boolean"},
			"category": map[string]string{"type": "string"},
		},
		"required": []string{"name", "price", "in_stock", "category"},
	}

	var product ProductInfo
	err := p.Structured().
		Model("gpt-3.5-turbo").
		Prompt("Extract product info: The new iPhone 15 Pro costs $999 and is currently in stock in the Electronics department").
		Schema(schema).
		Mode(types.StructuredModeJSON).
		GenerateAs(ctx, &product)

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Extracted Product: %+v\n", product)
}

func toolExample(ctx context.Context, p *prism.Prism) {
	// Define a weather tool
	weatherTool := types.NewTool(
		"get_current_weather",
		"Get the current weather in a given location",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]interface{}{
					"type":        "string",
					"description": "The city and state, e.g. San Francisco, CA",
				},
				"unit": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"celsius", "fahrenheit"},
					"description": "The unit of temperature",
				},
			},
			"required": []string{"location"},
		},
	)

	response, err := p.Text().
		Model("gpt-3.5-turbo").
		Prompt("What's the weather like in New York?").
		Tools(*weatherTool).
		Generate(ctx)

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	if len(response.ToolCalls) > 0 {
		fmt.Println("Model wants to call tools:")
		for _, call := range response.ToolCalls {
			fmt.Printf("- Tool: %s\n", call.Function.Name)
			fmt.Printf("  Arguments: %s\n", call.Function.Arguments)
		}
	} else {
		fmt.Println("Response:", response.Text)
	}
}
