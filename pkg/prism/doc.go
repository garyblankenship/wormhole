// Package prism provides a unified interface for working with Large Language Models (LLMs).
//
// Prism offers a fluent builder pattern for constructing requests and supports multiple
// providers including OpenAI, Anthropic, and more.
//
// Basic usage:
//
//	p := prism.New(prism.Config{
//	    DefaultProvider: "openai",
//	    Providers: map[string]types.ProviderConfig{
//	        "openai": {
//	            APIKey: os.Getenv("OPENAI_API_KEY"),
//	        },
//	    },
//	})
//
//	response, err := p.Text().
//	    Model("gpt-4").
//	    Prompt("Hello, world!").
//	    Generate(context.Background())
//
// The package supports:
//   - Text generation with conversation history
//   - Streaming responses via Go channels
//   - Structured output with JSON schemas
//   - Embeddings generation
//   - Image generation
//   - Audio operations (TTS/STT)
//   - Tool/function calling
//
// For more examples, see the README.md file.
package prism
