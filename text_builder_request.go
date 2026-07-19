package wormhole

import (
	"github.com/garyblankenship/wormhole/v2/types"
)

// textIdempotencyRequest wraps a TextRequest with fallback routes for
// idempotency key derivation, and delegates GetProviderOptions() so the
// idempotency cache key can fold provider-specific options into the hash.
type textIdempotencyRequest struct {
	Request           *types.TextRequest `json:"request"`
	FallbackModels    []string           `json:"fallback_models,omitempty"`
	ProviderFallbacks []TextRoute        `json:"provider_fallbacks,omitempty"`
}

func (r textIdempotencyRequest) GetProviderOptions() map[string]any {
	return r.Request.GetProviderOptions()
}

// TextRoute identifies a provider and model for a text generation attempt.
type TextRoute struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// TextRequestBuilder builds text generation requests
type TextRequestBuilder struct {
	CommonBuilder
	request               *types.TextRequest
	toolExecutionOverride *bool    // Explicit WithToolsEnabled/WithToolsDisabled choice; nil = unset, use auto-detect default
	maxToolIterations     int      // Maximum number of tool execution rounds (default: 10)
	fallbackModels        []string // Models to try in order if primary fails
	providerFallbacks     []TextRoute
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
	b.request.Messages = types.CloneMessages(messages)
	return b
}

// AddMessage adds a message to the request
func (b *TextRequestBuilder) AddMessage(message types.Message) *TextRequestBuilder {
	b.request.Messages = append(b.request.Messages, types.CloneMessage(message))
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
	clonedProviderFallbacks := append([]TextRoute(nil), b.providerFallbacks...)

	var clonedOverride *bool
	if b.toolExecutionOverride != nil {
		v := *b.toolExecutionOverride
		clonedOverride = &v
	}

	return &TextRequestBuilder{
		CommonBuilder: CommonBuilder{
			wormhole: b.wormhole,
			provider: b.provider,
			baseURL:  b.baseURL,
		},
		request:               clonedRequest,
		toolExecutionOverride: clonedOverride,
		maxToolIterations:     b.maxToolIterations,
		fallbackModels:        clonedFallbacks,
		providerFallbacks:     clonedProviderFallbacks,
	}
}
