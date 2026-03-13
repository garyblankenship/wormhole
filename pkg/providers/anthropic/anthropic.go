package anthropic

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/providers"
	transform "github.com/garyblankenship/wormhole/pkg/providers/transform"
	"github.com/garyblankenship/wormhole/pkg/types"
)

const defaultBaseURL = "https://api.anthropic.com/v1"

// Provider implements the Anthropic provider
type Provider struct {
	*providers.BaseProvider
	requestBuilder       *providers.RequestBuilder
	responseTransform    *transform.ResponseTransform
	streamingTransformer *transform.StreamingTransformer
}

// New creates a new Anthropic provider
func New(config types.ProviderConfig) *Provider {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}

	factory := &providers.AuthStrategyFactory{}
	authStrategy := factory.CreateAuthStrategy("anthropic", config)

	return &Provider{
		BaseProvider:         providers.NewBaseProviderWithAuth("anthropic", config, nil, authStrategy, nil),
		requestBuilder:       providers.NewRequestBuilder(),
		responseTransform:    transform.NewResponseTransform(),
		streamingTransformer: transform.NewAnthropicStreamingTransformer(),
	}
}

// SupportedCapabilities returns the capabilities supported by Anthropic provider
func (p *Provider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{
		types.CapabilityText,
		types.CapabilityChat,
		types.CapabilityStructured,
		types.CapabilityStream,
		types.CapabilityFunctions,
	}
}

// Text generates a text response
func (p *Provider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	payload := p.buildMessagePayload(&request)

	url := p.GetBaseURL() + "/messages"

	var response messageResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	return p.transformTextResponse(&response), nil
}

// Stream generates a streaming text response
func (p *Provider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.StreamChunk, error) {
	payload := p.buildMessagePayload(&request)
	payload["stream"] = true

	url := p.GetBaseURL() + "/messages"

	body, err := p.StreamRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	return utils.ProcessStream(body, p.parseStreamChunk, 100), nil
}

// Structured generates a structured response
func (p *Provider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	// Anthropic uses tool calling for structured output
	textRequest := types.TextRequest{
		BaseRequest:  request.BaseRequest,
		Messages:     request.Messages,
		SystemPrompt: request.SystemPrompt,
	}

	// Create a tool from the schema
	schemaBytes, err := json.Marshal(request.Schema)
	if err != nil {
		return nil, p.RequestError("failed to marshal schema", err)
	}
	tool, err := p.schemaToTool(json.RawMessage(schemaBytes), request.SchemaName)
	if err != nil {
		return nil, err
	}

	textRequest.Tools = []types.Tool{*tool}
	textRequest.ToolChoice = &types.ToolChoice{
		Type:     types.ToolChoiceTypeSpecific,
		ToolName: tool.Name,
	}

	response, err := p.Text(ctx, textRequest)
	if err != nil {
		return nil, err
	}

	// Extract structured data from tool call
	if len(response.ToolCalls) == 0 {
		return nil, p.ProviderError("no tool call in response")
	}

	var data any
	if response.ToolCalls[0].Function != nil {
		err = utils.UnmarshalAnthropicToolArgs(response.ToolCalls[0].Function.Arguments, &data)
	} else {
		// Fallback to Arguments field
		jsonBytes, _ := json.Marshal(response.ToolCalls[0].Arguments)
		err = utils.LenientUnmarshal(jsonBytes, &data)
	}
	if err != nil {
		return nil, p.RequestError("failed to parse structured response", err)
	}

	return &types.StructuredResponse{
		ID:      response.ID,
		Model:   response.Model,
		Data:    data,
		Usage:   response.Usage,
		Created: response.Created,
	}, nil
}

// Embeddings is not supported by Anthropic
func (p *Provider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	return nil, p.NotImplementedError("Embeddings")
}

// Audio is not supported by Anthropic
func (p *Provider) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	return nil, p.NotImplementedError("Audio")
}

// Images is not supported by Anthropic
func (p *Provider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	return nil, p.NotImplementedError("Images")
}

