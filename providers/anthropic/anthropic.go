package anthropic

import (
	"context"
	"encoding/json"
	"net/http"

	structured "github.com/garyblankenship/wormhole/v3/internal/structured"
	"github.com/garyblankenship/wormhole/v3/providers"
	providerstream "github.com/garyblankenship/wormhole/v3/providers/internal/stream"
	transform "github.com/garyblankenship/wormhole/v3/providers/internal/transform"
	"github.com/garyblankenship/wormhole/v3/types"
)

const defaultBaseURL = "https://api.anthropic.com/v1"

// Provider implements the Anthropic provider
type Provider struct {
	*providers.BaseProvider
	requestBuilder       *providers.RequestBuilder
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
	prepared, _, err := providers.PrepareMessages(request.Messages)
	if err != nil {
		return nil, err
	}
	payload, err := p.buildMessagePayload(&request, prepared)
	if err != nil {
		return nil, err
	}

	url := p.GetBaseURL() + "/messages"

	var response messageResponse
	metadata, err := p.DoRequestWithMetadata(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	resp := p.transformTextResponse(&response)
	resp.Provider = p.Name()
	resp.ResponseMetadata = metadata
	resp.Warnings = append(resp.Warnings, visibilityWarnings(request.Reasoning, payload)...)
	return resp, nil
}

// stampProvider sets Provider on the terminal chunk. Sole closer of out;
// exits when the upstream channel closes.
func (p *Provider) stampProvider(ctx context.Context, in <-chan types.StreamChunk, metadata *types.ResponseMetadata, warnings []types.ResponseWarning) <-chan types.StreamChunk {
	out := make(chan types.StreamChunk)
	go func() {
		defer close(out)
		first := true
		for chunk := range in {
			if first || chunk.IsDone() || chunk.Error != nil {
				chunk.ResponseMetadata = metadata
				chunk.Warnings = append(chunk.Warnings, warnings...)
				first = false
			}
			if chunk.Error != nil {
				chunk.Error = providers.WithResponseMetadata(chunk.Error, metadata)
			}
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
	prepared, _, err := providers.PrepareMessages(request.Messages)
	if err != nil {
		return nil, err
	}
	payload, err := p.buildMessagePayload(&request, prepared)
	if err != nil {
		return nil, err
	}
	payload["stream"] = true

	url := p.GetBaseURL() + "/messages"

	body, metadata, err := p.StreamRequestWithMetadata(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	return p.stampProvider(ctx, p.accumulatingStream(ctx, providerstream.ProcessSSE(ctx, body, p.parseStreamChunk, 100)), metadata, visibilityWarnings(request.Reasoning, payload)), nil
}

func (p *Provider) validateSamplingControls(request types.TextRequest) error {
	if r := request.Reasoning; r != nil && r.MaxTokens > 0 && ((r.Enabled != nil && !*r.Enabled) || r.Effort == types.ReasoningEffortNone) {
		return p.ValidationError("disabled reasoning conflicts with a positive thinking budget")
	}
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
	t, err := structured.Prepare(request)
	if err != nil {
		return nil, p.ValidationError(err.Error())
	}
	if request.Mode == types.StructuredModeJSON {
		t.ResponseFormat = map[string]any{"type": "json_schema", "json_schema": map[string]any{"schema": request.Schema}}
	}
	response, err := p.Text(ctx, t)
	if err != nil {
		return nil, err
	}
	return structured.Convert(request, response)
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
