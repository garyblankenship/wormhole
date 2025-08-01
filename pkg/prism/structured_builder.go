package prism

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prism-php/prism-go/pkg/types"
)

// StructuredRequestBuilder builds structured output requests
type StructuredRequestBuilder struct {
	prism    *Prism
	request  *types.StructuredRequest
	provider string
}

// Using sets the provider to use
func (b *StructuredRequestBuilder) Using(provider string) *StructuredRequestBuilder {
	b.provider = provider
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
	if b.request.Schema == nil {
		return nil, fmt.Errorf("no schema provided")
	}

	// Ensure we have a StructuredProvider
	structuredProvider, ok := provider.(types.StructuredProvider)
	if !ok {
		return nil, fmt.Errorf("provider %s does not support structured output", provider.Name())
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
