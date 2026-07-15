package anthropic

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/garyblankenship/wormhole/v2/providers"
	providerstream "github.com/garyblankenship/wormhole/v2/providers/internal/stream"
	transform "github.com/garyblankenship/wormhole/v2/providers/internal/transform"
	"github.com/garyblankenship/wormhole/v2/types"
)

const defaultBaseURL = "https://api.anthropic.com/v1"

// Provider implements the Anthropic provider
type Provider struct {
	*providers.BaseProvider
	requestBuilder       *providers.RequestBuilder
	responseTransform    *transform.ResponseTransform
	streamingTransformer *transform.StreamingTransformer
}

var _ types.Provider = (*Provider)(nil)

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
	if err := p.validateSamplingControls(request); err != nil {
		return nil, err
	}
	if _, _, err := providers.PrepareMessages(request.Messages); err != nil {
		return nil, err
	}
	payload := p.buildMessagePayload(&request)

	url := p.GetBaseURL() + "/messages"

	var response messageResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	resp := p.transformTextResponse(&response)
	resp.Provider = p.Name()
	return resp, nil
}

// stampProvider sets Provider on the terminal chunk. Sole closer of out;
// exits when the upstream channel closes.
func (p *Provider) stampProvider(ctx context.Context, in <-chan types.StreamChunk) <-chan types.StreamChunk {
	out := make(chan types.StreamChunk)
	go func() {
		defer close(out)
		for chunk := range in {
			if chunk.IsDone() {
				chunk.Provider = p.Name()
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// Stream generates a streaming text response
func (p *Provider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.StreamChunk, error) {
	if err := p.validateSamplingControls(request); err != nil {
		return nil, err
	}
	if _, _, err := providers.PrepareMessages(request.Messages); err != nil {
		return nil, err
	}
	payload := p.buildMessagePayload(&request)
	payload["stream"] = true

	url := p.GetBaseURL() + "/messages"

	body, err := p.StreamRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	return p.stampProvider(ctx, p.accumulatingStream(ctx, providerstream.ProcessSSE(ctx, body, p.parseStreamChunk, 100))), nil
}

func (p *Provider) validateSamplingControls(request types.TextRequest) error {
	if request.FrequencyPenalty != nil || request.PresencePenalty != nil || request.Seed != nil {
		return p.ValidationError("frequency_penalty, presence_penalty, and seed are not supported by Anthropic")
	}
	if request.ParallelToolCalls != nil && request.ToolChoice != nil && request.ToolChoice.Type == types.ToolChoiceTypeNone {
		return p.ValidationError("parallel_tool_calls cannot be used when Anthropic tool_choice is none")
	}
	return nil
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

	data, err := p.parseStructuredToolCall(response.ToolCalls[0])
	if err != nil {
		return nil, err
	}

	return &types.StructuredResponse{
		ID:      response.ID,
		Model:   response.Model,
		Data:    data,
		Usage:   response.Usage,
		Created: response.Created,
	}, nil
}

func (p *Provider) parseStructuredToolCall(toolCall types.ToolCall) (any, error) {
	var data any
	var err error
	if toolCall.Function != nil {
		err = unmarshalToolArgs(toolCall.Function.Arguments, &data)
	} else {
		jsonBytes, _ := json.Marshal(toolCall.Arguments)
		err = lenientUnmarshal(jsonBytes, &data)
	}
	if err != nil {
		return nil, p.RequestError("failed to parse structured response", err)
	}
	return data, nil
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
