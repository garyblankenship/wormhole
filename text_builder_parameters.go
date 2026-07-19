package wormhole

import (
	"github.com/garyblankenship/wormhole/v2/types"
)

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

// FrequencyPenalty adjusts how strongly repeated tokens are penalized.
// Supported range is -2.0 to 2.0.
func (b *TextRequestBuilder) FrequencyPenalty(penalty float32) *TextRequestBuilder {
	b.request.FrequencyPenalty = &penalty
	return b
}

// PresencePenalty adjusts how strongly tokens already present are penalized.
// Supported range is -2.0 to 2.0.
func (b *TextRequestBuilder) PresencePenalty(penalty float32) *TextRequestBuilder {
	b.request.PresencePenalty = &penalty
	return b
}

// Seed requests deterministic sampling from providers that support it.
func (b *TextRequestBuilder) Seed(seed int) *TextRequestBuilder {
	b.request.Seed = &seed
	return b
}

// ParallelToolCalls controls whether a provider may emit multiple tool calls
// in one model turn.
func (b *TextRequestBuilder) ParallelToolCalls(enabled bool) *TextRequestBuilder {
	b.request.ParallelToolCalls = &enabled
	return b
}

// Reasoning sets provider-neutral reasoning controls for models that support
// thinking or effort parameters. ProviderOptions can still override provider
// wire fields for advanced use.
func (b *TextRequestBuilder) Reasoning(reasoning types.Reasoning) *TextRequestBuilder {
	b.request.Reasoning = &reasoning
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
	b.request.Tools = types.CloneTools(tools)
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
	b.request.ResponseFormat = types.CloneValue(format)
	return b
}

// ProviderOptions sets provider-specific options
func (b *TextRequestBuilder) ProviderOptions(options map[string]any) *TextRequestBuilder {
	b.request.ProviderOptions = types.CloneMap(options)
	return b
}

// ==================== Tool Execution Configuration ====================

// WithToolsEnabled enables automatic tool execution.
// When enabled, the SDK will automatically execute tools when the model requests them,
// send results back, and continue the conversation until a final response is received.
//
// This is the default behavior when tools are registered on the client.
func (b *TextRequestBuilder) WithToolsEnabled() *TextRequestBuilder {
	enabled := true
	b.toolExecutionOverride = &enabled
	return b
}

// WithToolsDisabled disables automatic tool execution.
// When disabled, tool calls will be returned in the response and the caller
// must manually execute them and send results back.
func (b *TextRequestBuilder) WithToolsDisabled() *TextRequestBuilder {
	disabled := false
	b.toolExecutionOverride = &disabled
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

// WithProviderFallback sets provider/model routes to try after the primary
// model and any same-provider models configured with WithFallback.
func (b *TextRequestBuilder) WithProviderFallback(routes ...TextRoute) *TextRequestBuilder {
	b.providerFallbacks = routes
	return b
}
