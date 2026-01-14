package main

import (
	"context"
	"fmt"
	"log"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

// QuickExample demonstrates the simplified QuickOpenRouter() API
func QuickExample() {
	// This is what the user requested - super simple setup
	client, err := wormhole.QuickOpenRouter() // Uses OPENROUTER_API_KEY env var
	if err != nil {
		log.Printf("Failed to create OpenRouter client: %v", err)
		return
	}

	ctx := context.Background()

	// Test with GPT-5-mini as requested by the user
	response, err := client.Text().
		Model("openai/gpt-5-mini").
		Prompt("test").
		Generate(ctx)

	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	fmt.Printf("GPT-5-mini response: %s\n", response.Text)

	// Test model comparison as mentioned in user feedback
	models := []string{
		"openai/gpt-5-mini",
		"anthropic/claude-sonnet-4-5",
		"meta-llama/llama-3.1-8b-instruct",
	}

	for _, model := range models {
		response, err := client.Text().
			Model(model).
			Prompt("Explain AI in one sentence").
			Generate(ctx)

		if err != nil {
			fmt.Printf("❌ %s failed: %v\n", model, err)
			continue
		}

		fmt.Printf("✅ %s: %s\n", model, response.Text)
	}
}

// Uncomment to run this example:
// func main() {
//     QuickExample()
// }
