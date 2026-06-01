package wormhole

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	clonedRequest := cloneTextRequest(b.request)

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
	baseRequest := cloneTextRequest(b.request)
	prepareTextExecutionRequest(baseRequest)

	if len(baseRequest.Messages) == 0 {
		return nil, types.ErrInvalidRequest.WithDetails("no messages provided")
	}
	if baseRequest.Model == "" {
		return nil, types.ErrInvalidRequest.WithDetails("no model specified")
	}

	// Build list of models to try (primary + fallbacks)
	modelsToTry := make([]string, 0, 1+len(b.fallbackModels))
	modelsToTry = append(modelsToTry, baseRequest.Model)
	modelsToTry = append(modelsToTry, b.fallbackModels...)
	idempotencyRequest := struct {
		Request        *types.TextRequest `json:"request"`
		FallbackModels []string           `json:"fallback_models,omitempty"`
	}{
		Request:        baseRequest,
		FallbackModels: append([]string(nil), b.fallbackModels...),
	}

	return executeTrackedRequest(ctx, b.getWormhole(), b.idempotencyScope("text.generate"), idempotencyRequest, func(ctx context.Context) (*types.TextResponse, error) {
		provider, release, err := b.getProviderWithBaseURL()
		if err != nil {
			return nil, err
		}
		defer release()

		var lastErr error
		wormhole := b.getWormhole()
		for attempt, model := range modelsToTry {
			request := cloneTextRequest(baseRequest)
			request.Model = model
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.generate",
				Phase:     AttemptStarted,
				Provider:  provider.Name(),
				Model:     model,
				Attempt:   attempt + 1,
				Fallback:  attempt > 0,
			})

			resp, err := b.executeGenerate(ctx, provider, request)
			if err == nil {
				wormhole.emitAttempt(ctx, AttemptEvent{
					Operation: "text.generate",
					Phase:     AttemptSuccess,
					Provider:  provider.Name(),
					Model:     model,
					Attempt:   attempt + 1,
					Fallback:  attempt > 0,
				})
				return resp, nil
			}
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.generate",
				Phase:     AttemptError,
				Provider:  provider.Name(),
				Model:     model,
				Attempt:   attempt + 1,
				Fallback:  attempt > 0,
				Error:     err,
			})
			lastErr = err
		}

		return nil, lastErr
	})
}

// executeGenerate performs the actual generation with the current request settings
func (b *TextRequestBuilder) executeGenerate(ctx context.Context, provider types.Provider, request *types.TextRequest) (*types.TextResponse, error) {
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
		return executor.ExecuteWithTools(ctx, *request, provider, maxIterations)
	}

	// Standard execution without automatic tool handling

	// Apply type-safe middleware chain if configured
	if wormhole.providerMiddleware != nil {
		handler := wormhole.providerMiddleware.ApplyText(provider.Text)
		return handler(ctx, *request)
	}

	// No middleware configured, use provider directly
	return provider.Text(ctx, *request)
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
	baseRequest := cloneTextRequest(b.request)
	prepareTextExecutionRequest(baseRequest)

	if len(baseRequest.Messages) == 0 {
		return nil, types.ErrInvalidRequest.WithDetails("no messages provided")
	}
	if baseRequest.Model == "" {
		return nil, types.ErrInvalidRequest.WithDetails("no model specified")
	}

	modelsToTry := make([]string, 0, 1+len(b.fallbackModels))
	modelsToTry = append(modelsToTry, baseRequest.Model)
	modelsToTry = append(modelsToTry, b.fallbackModels...)

	if !b.getWormhole().trackRequest() {
		return nil, fmt.Errorf("client is shutting down")
	}

	provider, release, err := b.getProviderWithBaseURL()
	if err != nil {
		b.getWormhole().untrackRequest()
		return nil, err
	}

	// Let the provider handle model validation at request time
	// Provider handles all model validation and constraints
	stream := make(chan types.StreamChunk)
	go b.streamWithFallback(ctx, provider, release, baseRequest, modelsToTry, stream)
	return stream, nil
}

func (b *TextRequestBuilder) streamWithFallback(ctx context.Context, provider types.Provider, release func(), baseRequest *types.TextRequest, modelsToTry []string, out chan<- types.StreamChunk) {
	defer close(out)
	defer b.getWormhole().untrackRequest()
	defer release()

	var failures []string
	wormhole := b.getWormhole()
	for attempt, model := range modelsToTry {
		request := cloneTextRequest(baseRequest)
		request.Model = model
		wormhole.emitAttempt(ctx, AttemptEvent{
			Operation: "text.stream",
			Phase:     AttemptStarted,
			Provider:  provider.Name(),
			Model:     model,
			Attempt:   attempt + 1,
			Fallback:  attempt > 0,
			Stream:    true,
		})

		attemptCtx, cancelAttempt := context.WithCancel(ctx)
		stream, err := b.openStream(attemptCtx, provider, request)
		if err != nil {
			cancelAttempt()
			failures = append(failures, fmt.Sprintf("%s: %v", model, err))
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.stream",
				Phase:     AttemptError,
				Provider:  provider.Name(),
				Model:     model,
				Attempt:   attempt + 1,
				Fallback:  attempt > 0,
				Stream:    true,
				Error:     err,
			})
			if ctx.Err() != nil {
				return
			}
			continue
		}

		emitted, retry, err := forwardStreamWithFirstChunkSafety(ctx, cancelAttempt, out, stream)
		cancelAttempt()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", model, err))
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.stream",
				Phase:     AttemptError,
				Provider:  provider.Name(),
				Model:     model,
				Attempt:   attempt + 1,
				Fallback:  attempt > 0,
				Stream:    true,
				Error:     err,
			})
		}
		if emitted {
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.stream",
				Phase:     AttemptSuccess,
				Provider:  provider.Name(),
				Model:     model,
				Attempt:   attempt + 1,
				Fallback:  attempt > 0,
				Stream:    true,
			})
		}
		if emitted || !retry {
			return
		}
		if ctx.Err() != nil {
			return
		}
	}

	if ctx.Err() != nil {
		return
	}
	sendStreamChunk(ctx, out, types.StreamChunk{
		Error: fmt.Errorf("all stream attempts failed before emitting a chunk: %s", strings.Join(failures, "; ")),
	})
}

func (b *TextRequestBuilder) openStream(ctx context.Context, provider types.Provider, request *types.TextRequest) (<-chan types.StreamChunk, error) {
	var stream <-chan types.StreamChunk
	var err error

	if b.getWormhole().providerMiddleware != nil {
		handler := b.getWormhole().providerMiddleware.ApplyStream(provider.Stream)
		stream, err = handler(ctx, *request)
	} else {
		stream, err = provider.Stream(ctx, *request)
	}
	if err != nil {
		return nil, err
	}

	// Apply per-chunk idle timeout if configured.
	if timeout := b.getWormhole().config.StreamIdleTimeout; timeout > 0 {
		stream = applyStreamIdleTimeout(stream, timeout)
	}
	return stream, nil
}

func forwardStreamWithFirstChunkSafety(ctx context.Context, cancelAttempt context.CancelFunc, out chan<- types.StreamChunk, stream <-chan types.StreamChunk) (emitted bool, retry bool, err error) {
	for {
		select {
		case <-ctx.Done():
			return false, false, ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				if !emitted {
					return false, true, fmt.Errorf("stream closed before first chunk")
				}
				return true, false, nil
			}
			if !emitted && chunk.HasError() {
				cancelAttempt()
				go drainStream(ctx, stream)
				return false, true, chunk.Error
			}
			emitted = true
			if !sendStreamChunk(ctx, out, chunk) {
				return true, false, ctx.Err()
			}
			if chunk.HasError() {
				return true, false, chunk.Error
			}
		}
	}
}

func sendStreamChunk(ctx context.Context, out chan<- types.StreamChunk, chunk types.StreamChunk) bool {
	select {
	case out <- chunk:
		return true
	case <-ctx.Done():
		return false
	}
}

func drainStream(ctx context.Context, stream <-chan types.StreamChunk) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-stream:
			if !ok {
				return
			}
		}
	}
}

func cloneTextRequest(src *types.TextRequest) *types.TextRequest {
	if src == nil {
		return &types.TextRequest{}
	}

	cloned := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: src.Model,
		},
		SystemPrompt:   src.SystemPrompt,
		ResponseFormat: src.ResponseFormat,
	}

	cloneBaseRequestFields(&cloned.BaseRequest, &src.BaseRequest)
	if src.ToolChoice != nil {
		toolChoice := *src.ToolChoice
		cloned.ToolChoice = &toolChoice
	}
	if len(src.Messages) > 0 {
		cloned.Messages = make([]types.Message, len(src.Messages))
		copy(cloned.Messages, src.Messages)
	}
	if len(src.Tools) > 0 {
		cloned.Tools = make([]types.Tool, len(src.Tools))
		copy(cloned.Tools, src.Tools)
	}

	return cloned
}

func prepareTextExecutionRequest(request *types.TextRequest) {
	if request == nil {
		return
	}
	request.Messages = prepareExecutionMessages(request.SystemPrompt, request.Messages)
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
	var builder strings.Builder

	go func() {
		defer close(accumulated)
		for chunk := range stream {
			builder.WriteString(chunk.Content())
			accumulated <- chunk
		}
	}()

	return accumulated, func() string { return builder.String() }, nil
}

// applyStreamIdleTimeout wraps a provider stream with a per-chunk idle watchdog.
// If no chunk arrives within timeout, a typed timeout error is emitted and the
// source channel is drained so the provider goroutine can exit.
func applyStreamIdleTimeout(src <-chan types.StreamChunk, timeout time.Duration) <-chan types.StreamChunk {
	out := make(chan types.StreamChunk, cap(src))
	go func() {
		defer close(out)
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		for {
			select {
			case chunk, ok := <-src:
				if !ok {
					return
				}
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(timeout)
				out <- chunk
				if chunk.Error != nil {
					return
				}
			case <-timer.C:
				out <- types.StreamChunk{
					Error: fmt.Errorf("stream idle timeout: no chunk received within %s", timeout),
				}
				go func() {
					for range src {
					}
				}()
				return
			}
		}
	}()
	return out
}
