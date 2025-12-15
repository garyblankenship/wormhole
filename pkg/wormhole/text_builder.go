package wormhole

import (
	"context"
	"encoding/json"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// TextRequestBuilder builds text generation requests
type TextRequestBuilder struct {
	CommonBuilder
	request             *types.TextRequest
	enableToolExecution bool     // Whether to automatically execute tools
	maxToolIterations   int      // Maximum number of tool execution rounds (default: 10)
	fallbackModels      []string // Models to try in order if primary fails
}

// Using sets the provider to use
func (b *TextRequestBuilder) Using(provider string) *TextRequestBuilder {
	b.setProvider(provider)
	return b
}

// BaseURL sets a custom base URL for OpenAI-compatible APIs
func (b *TextRequestBuilder) BaseURL(url string) *TextRequestBuilder {
	b.setBaseURL(url)
	return b
}

// Model sets the model to use
func (b *TextRequestBuilder) Model(model string) *TextRequestBuilder {
	b.request.Model = model
	return b
}

// Messages sets the messages for the request
func (b *TextRequestBuilder) Messages(messages ...types.Message) *TextRequestBuilder {
	b.request.Messages = messages
	return b
}

// AddMessage adds a message to the request
func (b *TextRequestBuilder) AddMessage(message types.Message) *TextRequestBuilder {
	b.request.Messages = append(b.request.Messages, message)
	return b
}

// Conversation sets messages from a Conversation builder.
// This is the recommended way to build multi-turn conversations.
//
// Example:
//
//	conv := types.NewConversation().
//	    System("You are helpful").
//	    User("What is Go?").
//	    Assistant("Go is a programming language").
//	    User("What makes it good?")
//
//	response, _ := client.Text().Conversation(conv).Generate(ctx)
func (b *TextRequestBuilder) Conversation(conv *types.Conversation) *TextRequestBuilder {
	if conv != nil {
		// Extract system message if present and set as SystemPrompt
		if sysMsg := conv.SystemMessage(); sysMsg != nil {
			// Get content as string - SystemMessage stores Content as string
			if content, ok := sysMsg.GetContent().(string); ok {
				b.request.SystemPrompt = content
			}
			// Use messages without system (to avoid duplication)
			b.request.Messages = conv.WithoutSystem().Messages()
		} else {
			b.request.Messages = conv.Messages()
		}
	}
	return b
}

// Prompt sets a simple user prompt (convenience method)
func (b *TextRequestBuilder) Prompt(prompt string) *TextRequestBuilder {
	b.request.Messages = []types.Message{
		types.NewUserMessage(prompt),
	}
	return b
}

// SystemPrompt sets the system prompt
func (b *TextRequestBuilder) SystemPrompt(prompt string) *TextRequestBuilder {
	b.request.SystemPrompt = prompt
	return b
}

// Clone creates a deep copy of the builder with all settings preserved.
// This allows you to create variations from a base configuration.
//
// Example:
//
//	base := client.Text().Model("gpt-4o").Temperature(0.7)
//	resp1, _ := base.Clone().Prompt("Question 1").Generate(ctx)
//	resp2, _ := base.Clone().Prompt("Question 2").Generate(ctx)
func (b *TextRequestBuilder) Clone() *TextRequestBuilder {
	// Clone the request
	clonedRequest := getTextRequest()
	clonedRequest.Model = b.request.Model
	clonedRequest.SystemPrompt = b.request.SystemPrompt
	clonedRequest.ResponseFormat = b.request.ResponseFormat

	// Clone pointer fields
	if b.request.Temperature != nil {
		temp := *b.request.Temperature
		clonedRequest.Temperature = &temp
	}
	if b.request.TopP != nil {
		topP := *b.request.TopP
		clonedRequest.TopP = &topP
	}
	if b.request.MaxTokens != nil {
		maxTokens := *b.request.MaxTokens
		clonedRequest.MaxTokens = &maxTokens
	}
	if b.request.PresencePenalty != nil {
		pp := *b.request.PresencePenalty
		clonedRequest.PresencePenalty = &pp
	}
	if b.request.FrequencyPenalty != nil {
		fp := *b.request.FrequencyPenalty
		clonedRequest.FrequencyPenalty = &fp
	}
	if b.request.Seed != nil {
		seed := *b.request.Seed
		clonedRequest.Seed = &seed
	}
	if b.request.ToolChoice != nil {
		tc := *b.request.ToolChoice
		clonedRequest.ToolChoice = &tc
	}

	// Clone slices
	if len(b.request.Messages) > 0 {
		clonedRequest.Messages = make([]types.Message, len(b.request.Messages))
		copy(clonedRequest.Messages, b.request.Messages)
	}
	if len(b.request.Stop) > 0 {
		clonedRequest.Stop = make([]string, len(b.request.Stop))
		copy(clonedRequest.Stop, b.request.Stop)
	}
	if len(b.request.Tools) > 0 {
		clonedRequest.Tools = make([]types.Tool, len(b.request.Tools))
		copy(clonedRequest.Tools, b.request.Tools)
	}
	if len(b.request.ProviderOptions) > 0 {
		clonedRequest.ProviderOptions = make(map[string]any)
		for k, v := range b.request.ProviderOptions {
			clonedRequest.ProviderOptions[k] = v
		}
	}

	// Clone fallbackModels slice
	var clonedFallbacks []string
	if len(b.fallbackModels) > 0 {
		clonedFallbacks = make([]string, len(b.fallbackModels))
		copy(clonedFallbacks, b.fallbackModels)
	}

	return &TextRequestBuilder{
		CommonBuilder: CommonBuilder{
			wormhole: b.wormhole,
			provider: b.provider,
			baseURL:  b.baseURL,
		},
		request:             clonedRequest,
		enableToolExecution: b.enableToolExecution,
		maxToolIterations:   b.maxToolIterations,
		fallbackModels:      clonedFallbacks,
	}
}

// Temperature sets the sampling temperature for randomness in outputs.
// Range: 0.0 to 2.0 (provider-dependent). Lower values (0.0-0.3) produce
// focused, deterministic outputs. Higher values (0.7-1.0) increase creativity.
// Default varies by model. Cannot be used together with TopP on some providers.
//
// Typical values:
//   - 0.0: Deterministic (always same output for same input)
//   - 0.3: Focused but with slight variation
//   - 0.7: Balanced creativity (common default)
//   - 1.0: High creativity
//   - 1.5+: Very random, may produce incoherent output
func (b *TextRequestBuilder) Temperature(temp float32) *TextRequestBuilder {
	b.request.Temperature = &temp
	return b
}

// MaxTokens sets the maximum number of tokens to generate in the response.
// This limits output length and controls costs. One token is roughly 4 characters
// or 0.75 words in English. Setting this too low may cause truncated responses.
//
// Common limits by provider:
//   - OpenAI GPT-4o: up to 16,384 output tokens
//   - Anthropic Claude: up to 8,192 output tokens
//   - Gemini: up to 8,192 output tokens
//
// If not set, the model uses its default (often 1024-4096 tokens).
func (b *TextRequestBuilder) MaxTokens(tokens int) *TextRequestBuilder {
	b.request.MaxTokens = &tokens
	return b
}

// TopP sets nucleus sampling probability mass.
// Range: 0.0 to 1.0. The model considers tokens comprising the top P probability
// mass. Lower values (0.1) make output more focused; higher values (0.9) allow
// more diversity. Default is typically 1.0 (consider all tokens).
//
// Note: Using both Temperature and TopP is generally discouraged.
// Choose one approach for controlling randomness.
func (b *TextRequestBuilder) TopP(topP float32) *TextRequestBuilder {
	b.request.TopP = &topP
	return b
}

// Stop sets sequences that will halt generation when encountered.
// The model stops generating when it produces any of these sequences.
// Useful for controlling output format or preventing runaway generation.
//
// Example:
//
//	builder.Stop("\n\n", "END", "```")  // Stop at double newline, "END", or code fence
func (b *TextRequestBuilder) Stop(sequences ...string) *TextRequestBuilder {
	b.request.Stop = sequences
	return b
}

// Tools sets the tools available to the model
func (b *TextRequestBuilder) Tools(tools ...types.Tool) *TextRequestBuilder {
	b.request.Tools = tools
	return b
}

// ToolChoice sets how the model should use tools
func (b *TextRequestBuilder) ToolChoice(choice any) *TextRequestBuilder {
	if tc, ok := choice.(*types.ToolChoice); ok {
		b.request.ToolChoice = tc
	} else if str, ok := choice.(string); ok {
		b.request.ToolChoice = &types.ToolChoice{Type: types.ToolChoiceType(str)}
	}
	return b
}

// ResponseFormat sets the response format
func (b *TextRequestBuilder) ResponseFormat(format any) *TextRequestBuilder {
	b.request.ResponseFormat = format
	return b
}

// ProviderOptions sets provider-specific options
func (b *TextRequestBuilder) ProviderOptions(options map[string]any) *TextRequestBuilder {
	b.request.ProviderOptions = options
	return b
}

// ==================== Tool Execution Configuration ====================

// WithToolsEnabled enables automatic tool execution.
// When enabled, the SDK will automatically execute tools when the model requests them,
// send results back, and continue the conversation until a final response is received.
//
// This is the default behavior when tools are registered on the client.
func (b *TextRequestBuilder) WithToolsEnabled() *TextRequestBuilder {
	b.enableToolExecution = true
	return b
}

// WithToolsDisabled disables automatic tool execution.
// When disabled, tool calls will be returned in the response and the caller
// must manually execute them and send results back.
func (b *TextRequestBuilder) WithToolsDisabled() *TextRequestBuilder {
	b.enableToolExecution = false
	return b
}

// WithMaxToolIterations sets the maximum number of tool execution rounds.
// Default is 10. Set to 0 for unlimited (not recommended).
//
// This prevents infinite loops where the model keeps calling tools indefinitely.
func (b *TextRequestBuilder) WithMaxToolIterations(max int) *TextRequestBuilder {
	b.maxToolIterations = max
	return b
}

// WithFallback sets models to try in order if the primary model fails.
// This provides automatic resilience against model unavailability or rate limits.
//
// Example:
//
//	response, _ := client.Text().
//	    Model("gpt-4o").
//	    WithFallback("gpt-4o-mini", "gpt-4-turbo").
//	    Prompt("Complex task").
//	    Generate(ctx)  // Tries gpt-4o first, then fallbacks in order
func (b *TextRequestBuilder) WithFallback(models ...string) *TextRequestBuilder {
	b.fallbackModels = models
	return b
}

// Generate executes the request and returns a response
func (b *TextRequestBuilder) Generate(ctx context.Context) (*types.TextResponse, error) {
	provider, err := b.getProviderWithBaseURL()
	if err != nil {
		return nil, err
	}

	// Add system prompt as first message if set
	if b.request.SystemPrompt != "" {
		messages := []types.Message{types.NewSystemMessage(b.request.SystemPrompt)}
		messages = append(messages, b.request.Messages...)
		b.request.Messages = messages
	}

	// Validate request
	if len(b.request.Messages) == 0 {
		return nil, types.ErrInvalidRequest.WithDetails("no messages provided")
	}
	if b.request.Model == "" {
		return nil, types.ErrInvalidRequest.WithDetails("no model specified")
	}

	// Build list of models to try (primary + fallbacks)
	modelsToTry := []string{b.request.Model}
	modelsToTry = append(modelsToTry, b.fallbackModels...)

	var lastErr error
	for _, model := range modelsToTry {
		b.request.Model = model
		resp, err := b.executeGenerate(ctx, provider)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		// Continue to next model on error
	}

	return nil, lastErr
}

// executeGenerate performs the actual generation with the current request settings
func (b *TextRequestBuilder) executeGenerate(ctx context.Context, provider types.Provider) (*types.TextResponse, error) {
	// Check if we should enable automatic tool execution
	wormhole := b.getWormhole()
	shouldAutoExecuteTools := b.shouldAutoExecuteTools(wormhole)

	// If auto-execution is enabled, use the tool executor
	if shouldAutoExecuteTools {
		executor := NewToolExecutor(wormhole.toolRegistry)
		maxIterations := b.maxToolIterations
		if maxIterations == 0 {
			maxIterations = 10 // Default
		}

		// ExecuteWithTools will handle middleware internally by calling provider.Text
		// which goes through the middleware chain
		return executor.ExecuteWithTools(ctx, *b.request, provider, maxIterations)
	}

	// Standard execution without automatic tool handling

	// Apply type-safe middleware chain if configured
	if wormhole.providerMiddleware != nil {
		handler := wormhole.providerMiddleware.ApplyText(provider.Text)
		return handler(ctx, *b.request)
	}

	// Fallback to legacy middleware if configured
	if wormhole.middlewareChain != nil {
		handler := wormhole.middlewareChain.Apply(func(ctx context.Context, req any) (any, error) {
			textReq := req.(*types.TextRequest)
			return provider.Text(ctx, *textReq)
		})
		resp, err := handler(ctx, b.request)
		if err != nil {
			return nil, err
		}
		return resp.(*types.TextResponse), nil
	}

	return provider.Text(ctx, *b.request)
}

// shouldAutoExecuteTools determines if automatic tool execution should be enabled
func (b *TextRequestBuilder) shouldAutoExecuteTools(wormhole *Wormhole) bool {
	// Explicit disable takes precedence
	if !b.enableToolExecution && b.maxToolIterations == 0 {
		// User hasn't explicitly configured, check defaults
		// Auto-enable if:
		// 1. Tools are registered on the client AND
		// 2. No tools explicitly set on request (use registry tools)
		if wormhole.toolRegistry.Count() > 0 && len(b.request.Tools) == 0 {
			return true
		}
		return false
	}

	// User explicitly enabled
	return b.enableToolExecution
}

// Stream executes the request and returns a streaming response
func (b *TextRequestBuilder) Stream(ctx context.Context) (<-chan types.StreamChunk, error) {
	provider, err := b.getProviderWithBaseURL()
	if err != nil {
		return nil, err
	}

	// Add system prompt as first message if set
	if b.request.SystemPrompt != "" {
		messages := []types.Message{types.NewSystemMessage(b.request.SystemPrompt)}
		messages = append(messages, b.request.Messages...)
		b.request.Messages = messages
	}

	// Validate request
	if len(b.request.Messages) == 0 {
		return nil, types.ErrInvalidRequest.WithDetails("no messages provided")
	}
	if b.request.Model == "" {
		return nil, types.ErrInvalidRequest.WithDetails("no model specified")
	}

	// Let the provider handle model validation at request time

	// Provider handles all model validation and constraints

	// Apply type-safe middleware chain if configured
	if b.getWormhole().providerMiddleware != nil {
		handler := b.getWormhole().providerMiddleware.ApplyStream(provider.Stream)
		return handler(ctx, *b.request)
	}

	// Fallback to legacy middleware if configured
	if b.getWormhole().middlewareChain != nil {
		handler := b.getWormhole().middlewareChain.Apply(func(ctx context.Context, req any) (any, error) {
			textReq := req.(*types.TextRequest)
			return provider.Stream(ctx, *textReq)
		})
		resp, err := handler(ctx, b.request)
		if err != nil {
			return nil, err
		}
		return resp.(<-chan types.StreamChunk), nil
	}

	return provider.Stream(ctx, *b.request)
}

// ToJSON returns the request as JSON
func (b *TextRequestBuilder) ToJSON() (string, error) {
	jsonBytes, err := json.MarshalIndent(b.request, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// Validate checks the request configuration for errors before calling Generate().
// This enables fail-fast behavior to catch configuration issues early.
//
// Validates:
//   - Model is specified
//   - Messages are provided (either via Prompt, Messages, or Conversation)
//   - Temperature is in valid range (0.0-2.0)
//   - TopP is in valid range (0.0-1.0)
//   - MaxTokens is positive if specified
//
// Example:
//
//	builder := client.Text().Model("gpt-4o").Temperature(0.7)
//	if err := builder.Validate(); err != nil {
//	    log.Fatal("Invalid configuration:", err)
//	}
//	// Safe to call Generate()
//	resp, _ := builder.Prompt("Hello").Generate(ctx)
func (b *TextRequestBuilder) Validate() error {
	var errs types.ValidationErrors

	// Required fields
	if b.request.Model == "" {
		errs.Add("model", "required", nil, "model must be specified")
	}

	// Messages are checked but allowed to be empty at validation time
	// (they might be set later via Prompt() before Generate())

	// Temperature range
	if b.request.Temperature != nil {
		temp := *b.request.Temperature
		if temp < 0 || temp > 2 {
			errs.Add("temperature", "range", temp, "must be between 0.0 and 2.0")
		}
	}

	// TopP range
	if b.request.TopP != nil {
		topP := *b.request.TopP
		if topP < 0 || topP > 1 {
			errs.Add("top_p", "range", topP, "must be between 0.0 and 1.0")
		}
	}

	// MaxTokens positive
	if b.request.MaxTokens != nil && *b.request.MaxTokens <= 0 {
		errs.Add("max_tokens", "positive", *b.request.MaxTokens, "must be a positive integer")
	}

	// Frequency/Presence penalty ranges
	if b.request.FrequencyPenalty != nil {
		fp := *b.request.FrequencyPenalty
		if fp < -2 || fp > 2 {
			errs.Add("frequency_penalty", "range", fp, "must be between -2.0 and 2.0")
		}
	}
	if b.request.PresencePenalty != nil {
		pp := *b.request.PresencePenalty
		if pp < -2 || pp > 2 {
			errs.Add("presence_penalty", "range", pp, "must be between -2.0 and 2.0")
		}
	}

	return errs.Error()
}

// MustValidate calls Validate() and panics if validation fails.
// Use this for development/testing when invalid configuration should not occur.
//
// Example:
//
//	builder := client.Text().Model("gpt-4o").Temperature(0.7).MustValidate()
func (b *TextRequestBuilder) MustValidate() *TextRequestBuilder {
	if err := b.Validate(); err != nil {
		panic(err)
	}
	return b
}

// StreamAndAccumulate is a convenience method that streams the response while
// accumulating the full text. It returns both the channel for real-time processing
// and a function to get the complete response after streaming finishes.
//
// Example:
//
//	chunks, getResult, err := builder.StreamAndAccumulate(ctx)
//	if err != nil {
//	    return err
//	}
//	for chunk := range chunks {
//	    fmt.Print(chunk.Content())  // Print in real-time
//	}
//	fullText := getResult()  // Get complete accumulated text
func (b *TextRequestBuilder) StreamAndAccumulate(ctx context.Context) (<-chan types.StreamChunk, func() string, error) {
	stream, err := b.Stream(ctx)
	if err != nil {
		return nil, nil, err
	}

	accumulated := make(chan types.StreamChunk)
	var fullText string

	go func() {
		defer close(accumulated)
		for chunk := range stream {
			fullText += chunk.Content()
			accumulated <- chunk
		}
	}()

	return accumulated, func() string { return fullText }, nil
}
