package main

import (
	"fmt"

	"github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

func main() {
	// Initialize with mock configuration using functional options
	p := wormhole.New(
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithProviderConfig("openai", types.ProviderConfig{
			APIKey:  "test-key",
			BaseURL: "https://api.openai.com/v1",
		}),
	)

	// Show API structure
	fmt.Println("Wormhole Go - API Examples")
	fmt.Println("======================")
	fmt.Println()

	// Text generation
	fmt.Println("1. Text Generation:")
	fmt.Println(`response, err := p.Text().
    Model("gpt-5").
    Prompt("Hello world").
    Temperature(0.7).
    Generate(ctx)`)

	// Streaming
	fmt.Println("\n2. Streaming:")
	fmt.Println(`chunks, err := p.Text().
    Model("gpt-5").
    Prompt("Tell me a story").
    Stream(ctx)

for chunk := range chunks {
    fmt.Print(chunk.Delta)
}`)

	// Structured output
	fmt.Println("\n3. Structured Output:")
	fmt.Println(`var result MyStruct
err := p.Structured().
    Model("gpt-5").
    Prompt("Extract data...").
    Schema(schema).
    GenerateAs(ctx, &result)`)

	// Tool calling
	fmt.Println("\n4. Tool Calling:")
	fmt.Println(`response, err := p.Text().
    Model("gpt-5").
    Prompt("What's the weather?").
    Tools(weatherTool).
    Generate(ctx)`)

	// Use explicit messages for the JSON illustration. SystemPrompt is an
	// execution convenience field and is not included by ToJSON.
	req := p.Text().
		Model("gpt-5").
		Messages(
			types.NewSystemMessage("You are a helpful assistant"),
			types.NewUserMessage("Explain the builder pattern"),
		)

	json, _ := req.ToJSON()
	fmt.Println("\n5. Request JSON:")
	fmt.Println(json)

	fmt.Println("\n✓ Package successfully built and ready to use!")
}
