package main

import (
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func main() {
	// Initialize with mock configuration
	p := wormhole.New(wormhole.Config{
		DefaultProvider: "openai",
		Providers: map[string]types.ProviderConfig{
			"openai": {
				APIKey:  "test-key",
				BaseURL: "https://api.openai.com/v1",
			},
		},
	})

	// Show API structure
	fmt.Println("Prism Go - API Examples")
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

	// Show builder pattern
	req := p.Text().
		Model("gpt-5").
		SystemPrompt("You are a helpful assistant").
		Prompt("Explain the builder pattern")

	json, _ := req.ToJSON()
	fmt.Println("\n5. Request JSON:")
	fmt.Println(json)

	fmt.Println("\n✓ Package successfully built and ready to use!")
}
