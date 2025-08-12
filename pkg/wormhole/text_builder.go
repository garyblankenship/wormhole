package wormhole

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// TextRequestBuilder builds text generation requests
type TextRequestBuilder struct {
	prism    *Wormhole
	request  *types.TextRequest
	provider string
}

// Using sets the provider to use
func (b *TextRequestBuilder) Using(provider string) *TextRequestBuilder {
	b.provider = provider
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

// Temperature sets the temperature
func (b *TextRequestBuilder) Temperature(temp float32) *TextRequestBuilder {
	b.request.Temperature = &temp
	return b
}

// MaxTokens sets the maximum tokens
func (b *TextRequestBuilder) MaxTokens(tokens int) *TextRequestBuilder {
	b.request.MaxTokens = &tokens
	return b
}

// TopP sets the top_p parameter
func (b *TextRequestBuilder) TopP(topP float32) *TextRequestBuilder {
	b.request.TopP = &topP
	return b
}

// Stop sets stop sequences
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
func (b *TextRequestBuilder) ToolChoice(choice interface{}) *TextRequestBuilder {
	if tc, ok := choice.(*types.ToolChoice); ok {
		b.request.ToolChoice = tc
	} else if str, ok := choice.(string); ok {
		b.request.ToolChoice = &types.ToolChoice{Type: types.ToolChoiceType(str)}
	}
	return b
}

// ResponseFormat sets the response format
func (b *TextRequestBuilder) ResponseFormat(format interface{}) *TextRequestBuilder {
	b.request.ResponseFormat = format
	return b
}

// ProviderOptions sets provider-specific options
func (b *TextRequestBuilder) ProviderOptions(options map[string]interface{}) *TextRequestBuilder {
	b.request.ProviderOptions = options
	return b
}

// Generate executes the request and returns a response
func (b *TextRequestBuilder) Generate(ctx context.Context) (*types.TextResponse, error) {
	provider, err := b.prism.getProvider(b.provider)
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
		return nil, fmt.Errorf("no messages provided")
	}
	if b.request.Model == "" {
		return nil, fmt.Errorf("no model specified")
	}

	// Apply middleware chain if configured
	if b.prism.middlewareChain != nil {
		handler := b.prism.middlewareChain.Apply(func(ctx context.Context, req interface{}) (interface{}, error) {
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

// Stream executes the request and returns a streaming response
func (b *TextRequestBuilder) Stream(ctx context.Context) (<-chan types.StreamChunk, error) {
	provider, err := b.prism.getProvider(b.provider)
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
		return nil, fmt.Errorf("no messages provided")
	}
	if b.request.Model == "" {
		return nil, fmt.Errorf("no model specified")
	}

	// Apply middleware chain if configured
	if b.prism.middlewareChain != nil {
		handler := b.prism.middlewareChain.Apply(func(ctx context.Context, req interface{}) (interface{}, error) {
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
