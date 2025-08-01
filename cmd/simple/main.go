package main

import (
	"fmt"

	"github.com/prism-php/prism-go/pkg/prism"
	"github.com/prism-php/prism-go/pkg/types"
)

func main() {
	// Initialize with mock configuration
	p := prism.New(prism.Config{
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
    Model("gpt-4").
    Prompt("Hello world").
    Temperature(0.7).
    Generate(ctx)`)

	// Streaming
	fmt.Println("\n2. Streaming:")
	fmt.Println(`chunks, err := p.Text().
    Model("gpt-4").
    Prompt("Tell me a story").
    Stream(ctx)

for chunk := range chunks {
    fmt.Print(chunk.Delta)
}`)

	// Structured output
	fmt.Println("\n3. Structured Output:")
	fmt.Println(`var result MyStruct
err := p.Structured().
    Model("gpt-4").
    Prompt("Extract data...").
    Schema(schema).
    GenerateAs(ctx, &result)`)

	// Tool calling
	fmt.Println("\n4. Tool Calling:")
	fmt.Println(`response, err := p.Text().
    Model("gpt-4").
    Prompt("What's the weather?").
    Tools(weatherTool).
    Generate(ctx)`)

	// Show builder pattern
	req := p.Text().
		Model("gpt-4").
		SystemPrompt("You are a helpful assistant").
		Prompt("Explain the builder pattern")

	json, _ := req.ToJSON()
	fmt.Println("\n5. Request JSON:")
	fmt.Println(json)

	fmt.Println("\n✓ Package successfully built and ready to use!")
}
