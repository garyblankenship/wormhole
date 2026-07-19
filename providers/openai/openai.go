package openai

import (
	"context"
	"net/http"

	"github.com/garyblankenship/wormhole/v2/providers"
	providerstream "github.com/garyblankenship/wormhole/v2/providers/internal/stream"
	transform "github.com/garyblankenship/wormhole/v2/providers/internal/transform"
	"github.com/garyblankenship/wormhole/v2/types"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
)

// Provider implements the OpenAI provider
type Provider struct {
	*providers.BaseProvider
	requestBuilder       *providers.RequestBuilder
	responseTransform    *transform.ResponseTransform
	streamingTransformer *transform.StreamingTransformer
}

var _ types.Provider = (*Provider)(nil)

// New creates a new OpenAI provider
func New(config types.ProviderConfig) *Provider {
	return NewWithName("openai", config)
}

// NewWithName creates an OpenAI-compatible provider with a caller-visible provider name.
func NewWithName(name string, config types.ProviderConfig) *Provider {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}

	return &Provider{
		BaseProvider:         providers.NewBaseProvider(name, config),
		requestBuilder:       providers.NewRequestBuilder(),
		responseTransform:    transform.NewResponseTransform(),
		streamingTransformer: transform.NewOpenAIStreamingTransformer(),
	}
}

// chatCompletionsURL returns the chat-completions endpoint, honoring a
// configured ChatPath override (empty = the OpenAI default).
func (p *Provider) chatCompletionsURL() string {
	path := p.Config.ChatPath
	if path == "" {
		path = "/chat/completions"
	}
	return p.GetBaseURL() + path
}

// responsesURL returns the Responses API endpoint, honoring a configured
// ResponsesPath override (empty = the OpenAI default).
func (p *Provider) responsesURL() string {
	path := p.Config.ResponsesPath
	if path == "" {
		path = "/responses"
	}
	return p.GetBaseURL() + path
}

// imagesURL returns the image-generation endpoint, honoring a configured
// ImagePath override (empty = the OpenAI default).
func (p *Provider) imagesURL() string {
	path := p.Config.ImagePath
	if path == "" {
		path = "/images/generations"
	}
	return p.GetBaseURL() + path
}

// SupportedCapabilities returns the capabilities supported by OpenAI provider
func (p *Provider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{
		types.CapabilityText,
		types.CapabilityChat,
		types.CapabilityStructured,
		types.CapabilityEmbeddings,
		types.CapabilityAudio,
		types.CapabilityImages,
		types.CapabilityStream,
		types.CapabilityFunctions,
	}
}

// Text generates a text response
func (p *Provider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	if _, _, err := providers.PrepareMessages(request.Messages); err != nil {
		return nil, err
	}
	if p.Config.UseResponsesAPI {
		return p.responsesText(ctx, request)
	}

	payload := p.buildChatPayload(&request)

	url := p.chatCompletionsURL()

	var response chatCompletionResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	textResponse := p.transformTextResponse(&response)
	textResponse.Provider = p.Name()

	// Validate response has content to prevent silent failures
	if textResponse.Text == "" && len(textResponse.ToolCalls) == 0 {
		return nil, p.ProviderError("received empty response from OpenAI API", "no content or tool calls returned")
	}

	return textResponse, nil
}

// Stream generates a streaming text response
func (p *Provider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	if _, _, err := providers.PrepareMessages(request.Messages); err != nil {
		return nil, err
	}
	if p.Config.UseResponsesAPI {
		return p.responsesStream(ctx, request)
	}

	payload := p.buildChatPayload(&request)
	payload["stream"] = true
	// Ask OpenAI to emit a final usage-bearing chunk on streamed responses;
	// without this, streamed Usage is always nil.
	payload["stream_options"] = map[string]any{"include_usage": true}

	url := p.chatCompletionsURL()

	body, err := p.StreamRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	return p.stampProvider(ctx, p.accumulatingStream(ctx, providerstream.ProcessSSE(ctx, body, p.parseStreamChunk, 100))), nil
}

// stampProvider sets Provider on the terminal chunk. Sole closer of out;
// exits when the upstream channel closes.
func (p *Provider) stampProvider(ctx context.Context, in <-chan types.TextChunk) <-chan types.TextChunk {
	out := make(chan types.TextChunk)
	go func() {
		defer close(out)
		for chunk := range in {
			if chunk.IsDone() {
				chunk.Provider = p.Name()
			}
			select {
			case <-ctx.Done():
				return
			default:
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
