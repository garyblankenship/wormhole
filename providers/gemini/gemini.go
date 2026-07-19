package gemini

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v2/providers"
	transform "github.com/garyblankenship/wormhole/v2/providers/internal/transform"
	"github.com/garyblankenship/wormhole/v2/types"
)

const (
	defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
)

// Gemini provider implementation
type Gemini struct {
	*providers.BaseProvider
	requestBuilder       *providers.RequestBuilder
	responseTransform    *transform.ResponseTransform
	streamingTransformer *transform.StreamingTransformer
}

var _ types.Provider = (*Gemini)(nil)

// New creates a new Gemini provider
func New(apiKey string, config types.ProviderConfig) *Gemini {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}

	// Gemini authenticates via a ?key= URL query param. Resolve the key (APIKeys[0]
	// when only APIKeys is set) and route it through config.APIKey +
	// QueryParamAuthStrategy so the HTTP wrapper applies ?key= on every attempt AND
	// re-derives it after a 429: the wrapper extracts the failed key from the query
	// param, advances the keyPool, and re-applies the new ?key= on the retry. Baking
	// the key into the URL string (the old approach) made mid-flight key rotation a
	// no-op because the retried request reused the original key.
	if apiKey == "" {
		apiKey = config.EffectiveAPIKey()
	}
	authStrategy := providers.AuthStrategy(&providers.NoAuthStrategy{})
	if apiKey != "" {
		config.APIKey = apiKey
		authStrategy = providers.NewQueryParamAuthStrategy("key")
	}

	return &Gemini{
		BaseProvider:         providers.NewBaseProviderWithAuth("gemini", config, nil, authStrategy, nil),
		requestBuilder:       providers.NewRequestBuilder(),
		responseTransform:    transform.NewResponseTransform(),
		streamingTransformer: nil,
	}
}

// Name returns the provider name
func (g *Gemini) Name() string {
	return "gemini"
}

// SupportedCapabilities returns the capabilities supported by Gemini provider
func (g *Gemini) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{
		types.CapabilityText,
		types.CapabilityChat,
		types.CapabilityStructured,
		types.CapabilityEmbeddings,
		types.CapabilityImages,
		types.CapabilityStream,
		types.CapabilityFunctions,
	}
}

// Text generates text using Gemini models
func (g *Gemini) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	payload, err := g.buildTextPayload(request)
	if err != nil {
		return nil, err
	}

	modelName := normalizeModelResource(request.Model)
	endpoint := fmt.Sprintf("%s/models/%s:generateContent",
		g.GetBaseURL(),
		modelName,
	)

	var response geminiTextResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	resp, err := g.transformTextResponse(&response)
	if err != nil {
		return nil, err
	}
	resp.Provider = g.Name()
	return resp, nil
}

// stampProvider sets Provider on the terminal chunk. Sole closer of out;
// exits when the upstream channel closes.
func (g *Gemini) stampProvider(ctx context.Context, in <-chan types.TextChunk) <-chan types.TextChunk {
	out := make(chan types.TextChunk)
	go func() {
		defer close(out)
		for chunk := range in {
			if chunk.IsDone() {
				chunk.Provider = g.Name()
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

// Stream generates streaming text using Gemini models
func (g *Gemini) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	payload, err := g.buildTextPayload(request)
	if err != nil {
		return nil, err
	}

	modelName := normalizeModelResource(request.Model)
	// alt=sse is REQUIRED: streamGenerateContent defaults to a JSON-array stream,
	// but handleStream parses with an SSE scanner. Without it the live endpoint
	// returns an unparseable array. (Streaming endpoint only — not generateContent.)
	endpoint := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse",
		g.GetBaseURL(),
		modelName,
	)

	stream, err := g.StreamRequest(ctx, "POST", endpoint, payload)
	if err != nil {
		return nil, err
	}

	return g.stampProvider(ctx, g.handleStream(ctx, stream)), nil
}

// Structured generates structured output using Gemini models
func (g *Gemini) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	payload, err := g.buildStructuredPayload(request)
	if err != nil {
		return nil, err
	}

	modelName := normalizeModelResource(request.Model)
	endpoint := fmt.Sprintf("%s/models/%s:generateContent",
		g.GetBaseURL(),
		modelName,
	)

	var response geminiTextResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	return g.transformStructuredResponse(&response, request.Schema)
}

// Audio is not supported by Gemini
func (g *Gemini) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	return nil, g.NotImplementedError("Audio")
}

// Images generates images using Gemini's native generateContent endpoint.
func (g *Gemini) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	payload, err := g.buildImagesPayload(request)
	if err != nil {
		return nil, err
	}

	modelName := normalizeModelResource(request.Model)
	endpoint := fmt.Sprintf("%s/models/%s:generateContent",
		g.GetBaseURL(),
		modelName,
	)

	var response geminiTextResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	return g.transformImagesResponse(&response, request.Model)
}
