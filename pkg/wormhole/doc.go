// Package wormhole provides instant traversal to any Large Language Model universe.
//
// Wormhole bends spacetime to deliver sub-microsecond latency when connecting to LLMs,
// supporting multiple AI universes including OpenAI, Anthropic, Gemini, and more.
//
// Basic usage:
//
//	client := wormhole.New(
//	    wormhole.WithDefaultProvider("openai"),
//	    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
//	)
//
//	response, err := client.Text().
//	    Model("gpt-5").
//	    Prompt("Hello from the other side of spacetime!").
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
package wormhole
