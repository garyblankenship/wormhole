# Prism Go Examples

This directory contains examples demonstrating various features of the Prism Go package.

## Running Examples

All examples use environment variables for API keys:

```bash
export OPENAI_API_KEY="your-key"
export ANTHROPIC_API_KEY="your-key"
```

## Basic Examples

### Simple Text Generation
```bash
go run ../cmd/simple/main.go
```
Shows basic API usage patterns without making actual API calls.

### Full Feature Demo
```bash
go run ../cmd/example/main.go
```
Demonstrates:
- Text generation
- Conversations
- Streaming
- Structured output
- Tool/function calling

## Advanced Examples

### Streaming with Context
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

stream, err := client.Text().
    Model("gpt-4").
    Stream(ctx)
```

### Error Handling
```go
response, err := client.Text().Generate(ctx)
if err != nil {
    var prismErr types.PrismError
    if errors.As(err, &prismErr) {
        log.Printf("API Error: %s (Code: %s)", prismErr.Message, prismErr.Code)
    }
}
```

### Provider Switching
```go
// Use different providers for different tasks
summary, _ := client.Text().
    Using("openai").
    Model("gpt-3.5-turbo").
    Prompt("Summarize: " + longText).
    Generate(ctx)

analysis, _ := client.Text().
    Using("anthropic").
    Model("claude-3-sonnet-20240229").
    Prompt("Analyze: " + summary.Text).
    Generate(ctx)
```

## Creating Your Own Examples

1. Create a new Go file in your project
2. Import the package: `import "github.com/prism-php/prism-go/pkg/prism"`
3. Initialize the client with your configuration
4. Use the builder pattern to construct requests
5. Handle responses and errors appropriately

See the main README.md for more detailed documentation.