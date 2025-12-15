package wormhole

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// StructuredRequestBuilder builds structured output requests
type StructuredRequestBuilder struct {
	CommonBuilder
	request *types.StructuredRequest
}

// Using sets the provider to use
func (b *StructuredRequestBuilder) Using(provider string) *StructuredRequestBuilder {
	b.setProvider(provider)
	return b
}

// BaseURL sets a custom base URL for OpenAI-compatible APIs
func (b *StructuredRequestBuilder) BaseURL(url string) *StructuredRequestBuilder {
	b.setBaseURL(url)
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
func (b *StructuredRequestBuilder) Schema(schema any) *StructuredRequestBuilder {
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

	// Apply type-safe middleware chain if configured
	if b.getWormhole().providerMiddleware != nil {
		handler := b.getWormhole().providerMiddleware.ApplyStructured(provider.Structured)
		return handler(ctx, *b.request)
	}

	// Fallback to legacy middleware if configured
	if b.getWormhole().middlewareChain != nil {
		handler := b.getWormhole().middlewareChain.Apply(func(ctx context.Context, req any) (any, error) {
			structuredReq := req.(*types.StructuredRequest)
			return provider.Structured(ctx, *structuredReq)
		})
		resp, err := handler(ctx, b.request)
		if err != nil {
			return nil, err
		}
		return resp.(*types.StructuredResponse), nil
	}

	return provider.Structured(ctx, *b.request)
}

// GenerateAs executes the request and unmarshals the response into the provided type
func (b *StructuredRequestBuilder) GenerateAs(ctx context.Context, result any) error {
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

// Validate checks the request configuration for errors before calling Generate().
// This enables fail-fast behavior to catch configuration issues early.
//
// Validates:
//   - Model is specified
//   - Schema is provided
//   - Temperature is in valid range (0.0-2.0) if specified
//   - MaxTokens is positive if specified
//
// Example:
//
//	builder := client.Structured().Model("gpt-4o").Schema(mySchema)
//	if err := builder.Validate(); err != nil {
//	    log.Fatal("Invalid configuration:", err)
//	}
func (b *StructuredRequestBuilder) Validate() error {
	var errs types.ValidationErrors

	if b.request.Model == "" {
		errs.Add("model", "required", nil, "model must be specified")
	}

	if b.request.Schema == nil {
		errs.Add("schema", "required", nil, "schema must be specified for structured output")
	}

	if b.request.Temperature != nil {
		temp := *b.request.Temperature
		if temp < 0 || temp > 2 {
			errs.Add("temperature", "range", temp, "must be between 0.0 and 2.0")
		}
	}

	if b.request.MaxTokens != nil && *b.request.MaxTokens <= 0 {
		errs.Add("max_tokens", "positive", *b.request.MaxTokens, "must be a positive integer")
	}

	return errs.Error()
}

// MustValidate calls Validate() and panics if validation fails.
func (b *StructuredRequestBuilder) MustValidate() *StructuredRequestBuilder {
	if err := b.Validate(); err != nil {
		panic(err)
	}
	return b
}
