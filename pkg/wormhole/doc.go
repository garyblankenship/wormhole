// Package wormhole provides a unified SDK for interacting with multiple LLM providers.
//
// Wormhole abstracts away provider-specific implementations, allowing you to switch
// between OpenAI, Anthropic, Google Gemini, OpenRouter, and other providers with
// minimal code changes.
//
// # Quick Start
//
// Create a client and make a request:
//
//	client := wormhole.New(
//	    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
//	    wormhole.WithDefaultProvider("openai"),
//	)
//
//	response, err := client.Text().
//	    Model("gpt-4o").
//	    Prompt("Hello, world!").
//	    Generate(context.Background())
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(response.Text)
//
// # Supported Providers
//
// Built-in providers with dedicated configuration:
//   - OpenAI (GPT-4, GPT-4o, o1 family) via WithOpenAI()
//   - Anthropic (Claude 3.5, Claude 3 family) via WithAnthropic()
//   - Google Gemini (Gemini Pro, Gemini Flash) via WithGemini()
//   - Groq (fast inference) via WithGroq()
//   - Mistral via WithMistral()
//   - Ollama (local models) via WithOllama()
//
// OpenAI-compatible providers work via WithOpenAICompatible():
//   - OpenRouter (200+ models)
//   - Together AI
//   - Fireworks AI
//   - Any endpoint implementing the OpenAI API
//
// # Request Builders
//
// All requests use a fluent builder pattern:
//
//	// Text generation
//	client.Text().
//	    Model("gpt-4o").
//	    SystemPrompt("You are a helpful assistant").
//	    Prompt("Explain quantum computing").
//	    Temperature(0.7).
//	    MaxTokens(1000).
//	    Generate(ctx)
//
//	// Streaming
//	stream, _ := client.Text().
//	    Model("claude-3-5-sonnet").
//	    Prompt("Write a story").
//	    Stream(ctx)
//	for chunk := range stream {
//	    fmt.Print(chunk.Text)
//	}
//
//	// Structured output
//	client.Structured().
//	    Model("gpt-4o").
//	    Schema(mySchema).
//	    Prompt("Extract entities").
//	    Generate(ctx)
//
//	// Embeddings
//	client.Embeddings().
//	    Model("text-embedding-3-small").
//	    Input("Hello world").
//	    Generate(ctx)
//
// # Builder Validation
//
// Validate builder configuration before making requests:
//
//	builder := client.Text().
//	    Model("gpt-4o").
//	    Temperature(0.7).
//	    MaxTokens(1000)
//
//	if err := builder.Validate(); err != nil {
//	    // Get detailed field-level error information
//	    if vErr, ok := types.AsValidationError(err); ok {
//	        fmt.Printf("Field '%s' failed: %s\n", vErr.Field, vErr.Message)
//	    }
//	    return err
//	}
//
//	// Or use MustValidate for tests (panics on error)
//	client.Text().Model("gpt-4o").MustValidate().Prompt("Hi").Generate(ctx)
//
// # Response Accessors
//
// All response types have unified Content() method:
//
//	textResp, _ := client.Text().Model("gpt-4o").Prompt("Hi").Generate(ctx)
//	fmt.Println(textResp.Content())       // string
//	fmt.Println(textResp.HasToolCalls())  // bool
//	fmt.Println(textResp.IsComplete())    // bool (finished normally)
//
//	embResp, _ := client.Embeddings().Input("text").Generate(ctx)
//	vector := embResp.Content()   // first []float64
//	vector2 := embResp.Vector(1)  // get by index
//
//	for chunk := range stream {
//	    fmt.Print(chunk.Content())  // works for Text and Delta
//	    if chunk.IsDone() { break }
//	}
//
// # Provider Capabilities
//
// Check provider capabilities before using features:
//
//	caps := client.ProviderCapabilities("openai")
//	if caps.SupportsToolCalling() {
//	    client.RegisterTool(...)
//	}
//	if caps.Has(wormhole.CapabilityVision) {
//	    // safe to send images
//	}
//
// # Error Handling
//
// All errors are wrapped in WormholeError with typed error codes:
//
//	response, err := client.Text().Generate(ctx)
//	if err != nil {
//	    // Check error type
//	    if types.IsAuthError(err) {
//	        log.Fatal("Invalid API key")
//	    }
//	    if types.IsRateLimitError(err) {
//	        delay := types.GetRetryAfter(err)
//	        time.Sleep(delay)
//	        // retry...
//	    }
//
//	    // Get full error details
//	    var wormholeErr *types.WormholeError
//	    if errors.As(err, &wormholeErr) {
//	        fmt.Printf("Code: %s, Provider: %s\n", wormholeErr.Code, wormholeErr.Provider)
//	        if wormholeErr.IsRetryable() {
//	            // implement retry logic
//	        }
//	    }
//	}
//
// # Tool Calling / Function Calling
//
// Register tools at the client level for automatic execution:
//
//	client.RegisterTool(
//	    "get_weather",
//	    "Get current weather for a city",
//	    types.ObjectSchema{
//	        Type: "object",
//	        Properties: map[string]types.Schema{
//	            "city": types.StringSchema{Type: "string"},
//	        },
//	        Required: []string{"city"},
//	    },
//	    func(ctx context.Context, args map[string]any) (any, error) {
//	        city := args["city"].(string)
//	        return map[string]any{"temp": 72, "unit": "F"}, nil
//	    },
//	)
//
//	// Tools are automatically executed when the model requests them
//	response, _ := client.Text().
//	    Model("gpt-4o").
//	    Prompt("What's the weather in Paris?").
//	    Generate(ctx)
//
// # Dynamic Model Discovery
//
// Wormhole automatically discovers available models from configured providers:
//
//	// List available models
//	models, _ := client.ListAvailableModels("openai")
//	for _, m := range models {
//	    fmt.Printf("%s: %v\n", m.ID, m.Capabilities)
//	}
//
//	// Refresh model cache
//	client.RefreshModels()
//
// # Middleware
//
// Add cross-cutting concerns like logging, caching, and rate limiting:
//
//	client := wormhole.New(
//	    wormhole.WithOpenAI(apiKey),
//	    wormhole.WithMiddleware(
//	        middleware.NewLoggingMiddleware(logger),
//	        middleware.NewCacheMiddleware(cache),
//	        middleware.NewRateLimiterMiddleware(10, time.Second),
//	    ),
//	)
//
// # Provider Selection
//
// Switch providers per-request without reconfiguring:
//
//	// Use default provider
//	client.Text().Prompt("Hello").Generate(ctx)
//
//	// Override for specific request
//	client.Text().Using("anthropic").Prompt("Hello").Generate(ctx)
//
//	// Use any OpenAI-compatible endpoint
//	client.Text().
//	    BaseURL("https://openrouter.ai/api/v1").
//	    Model("anthropic/claude-3-opus").
//	    Prompt("Hello").
//	    Generate(ctx)
//
// # Testing
//
// Use the testing package for mocks:
//
//	import whtest "github.com/garyblankenship/wormhole/pkg/testing"
//
//	mock := whtest.NewMockProvider("test").
//	    WithTextResponse(types.TextResponse{Text: "mocked"})
//
//	// Use mock in tests via WithCustomProvider
package wormhole
