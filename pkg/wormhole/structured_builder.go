package wormhole

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// StructuredRequestBuilder builds structured output requests
type StructuredRequestBuilder struct {
	wormhole *Wormhole
	request  *types.StructuredRequest
	provider string
	baseURL  string
}

// Using sets the provider to use
func (b *StructuredRequestBuilder) Using(provider string) *StructuredRequestBuilder {
	b.provider = provider
	return b
}

// Provider sets the provider to use (alias for Using)
func (b *StructuredRequestBuilder) Provider(provider string) *StructuredRequestBuilder {
	b.provider = provider
	return b
}

// BaseURL sets a custom base URL for OpenAI-compatible APIs
func (b *StructuredRequestBuilder) BaseURL(url string) *StructuredRequestBuilder {
	b.baseURL = url
	return b
}

// Model sets the model to use
func (b *StructuredRequestBuilder) Model(model string) *StructuredRequestBuilder {
	b.request.Model = model
	return b
}

// Messages sets the messages for the request
func (b *StructuredRequestBuilder) Messages(messages ...types.Message) *StructuredRequestBuilder {
	b.request.Messages = messages
	return b
}

// AddMessage adds a message to the request
func (b *StructuredRequestBuilder) AddMessage(message types.Message) *StructuredRequestBuilder {
	b.request.Messages = append(b.request.Messages, message)
	return b
}

// Prompt sets a simple user prompt (convenience method)
func (b *StructuredRequestBuilder) Prompt(prompt string) *StructuredRequestBuilder {
	b.request.Messages = []types.Message{
		types.NewUserMessage(prompt),
	}
	return b
}

// SystemPrompt sets the system prompt
func (b *StructuredRequestBuilder) SystemPrompt(prompt string) *StructuredRequestBuilder {
	b.request.SystemPrompt = prompt
	return b
}

// Schema sets the JSON schema for the response
func (b *StructuredRequestBuilder) Schema(schema interface{}) *StructuredRequestBuilder {
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		// Store error to return during Generate
		b.request.Schema = nil
	} else {
		b.request.Schema = schemaBytes
	}
	return b
}

// SchemaName sets the name of the schema
func (b *StructuredRequestBuilder) SchemaName(name string) *StructuredRequestBuilder {
	b.request.SchemaName = name
	return b
}

// Mode sets the structured output mode
func (b *StructuredRequestBuilder) Mode(mode types.StructuredMode) *StructuredRequestBuilder {
	b.request.Mode = mode
	return b
}

// Temperature sets the temperature
func (b *StructuredRequestBuilder) Temperature(temp float32) *StructuredRequestBuilder {
	b.request.Temperature = &temp
	return b
}

// MaxTokens sets the maximum tokens
func (b *StructuredRequestBuilder) MaxTokens(tokens int) *StructuredRequestBuilder {
	b.request.MaxTokens = &tokens
	return b
}

// Generate executes the request and returns a structured response
func (b *StructuredRequestBuilder) Generate(ctx context.Context) (*types.StructuredResponse, error) {
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
		return nil, fmt.Errorf("no messages provided")
	}
	if b.request.Model == "" {
		return nil, fmt.Errorf("no model specified")
	}
	if b.request.Schema == nil {
		return nil, fmt.Errorf("no schema provided")
	}

	// Ensure we have a StructuredProvider
	structuredProvider, ok := provider.(types.StructuredProvider)
	if !ok {
		return nil, fmt.Errorf("provider %s does not support structured output", provider.Name())
	}

	// Apply middleware chain if configured
	if b.wormhole.middlewareChain != nil {
		handler := b.wormhole.middlewareChain.Apply(func(ctx context.Context, req interface{}) (interface{}, error) {
			structuredReq := req.(*types.StructuredRequest)
			return structuredProvider.Structured(ctx, *structuredReq)
		})
		resp, err := handler(ctx, b.request)
		if err != nil {
			return nil, err
		}
		return resp.(*types.StructuredResponse), nil
	}

	return structuredProvider.Structured(ctx, *b.request)
}

// GenerateAs executes the request and unmarshals the response into the provided type
func (b *StructuredRequestBuilder) GenerateAs(ctx context.Context, result interface{}) error {
	response, err := b.Generate(ctx)
	if err != nil {
		return err
	}

	// Marshal the response data to JSON then unmarshal into the result
	jsonBytes, err := json.Marshal(response.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal response data: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, result); err != nil {
		return fmt.Errorf("failed to unmarshal response data: %w", err)
	}

	return nil
}

// getProviderWithBaseURL gets the provider, creating a temporary one with custom baseURL if specified
func (b *StructuredRequestBuilder) getProviderWithBaseURL() (types.Provider, error) {
	// If no custom baseURL, use normal provider
	if b.baseURL == "" {
		return b.wormhole.getProvider(b.provider)
	}
	
	// Create a temporary OpenAI-compatible provider with custom baseURL
	providerName := b.provider
	if providerName == "" {
		providerName = b.wormhole.config.DefaultProvider
	}
	
	// Get existing provider config for API key
	var apiKey string
	if providerConfig, exists := b.wormhole.config.Providers[providerName]; exists {
		apiKey = providerConfig.APIKey
	}
	
	// Create temporary provider with custom baseURL
	tempConfig := types.ProviderConfig{
		APIKey:  apiKey,
		BaseURL: b.baseURL,
	}
	
	return b.wormhole.createOpenAICompatibleProvider(tempConfig)
}
