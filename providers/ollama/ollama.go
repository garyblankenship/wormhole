package ollama

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/garyblankenship/wormhole/v3/internal/structured"
	"github.com/garyblankenship/wormhole/v3/providers"
	providerstream "github.com/garyblankenship/wormhole/v3/providers/internal/stream"
	transform "github.com/garyblankenship/wormhole/v3/providers/internal/transform"
	"github.com/garyblankenship/wormhole/v3/types"
)

// No default base URLs - Ollama must be configured with explicit URL

// Provider implements the Ollama provider
type Provider struct {
	*providers.BaseProvider
	requestBuilder       *providers.RequestBuilder
	streamingTransformer *transform.StreamingTransformer
}

var _ types.Provider = (*Provider)(nil)

// New creates a new Ollama provider
func New(config types.ProviderConfig) (*Provider, error) {
	if config.BaseURL == "" {
		err := types.NewWormholeError(types.ErrorCodeValidation, "Ollama BaseURL is required", false)
		err.Details = "provide via config.BaseURL or environment variable"
		err.Provider = "ollama"
		return nil, err
	}

	return &Provider{
		BaseProvider:         providers.NewBaseProviderWithAuth("ollama", config, nil, &providers.NoAuthStrategy{}, nil),
		requestBuilder:       providers.NewRequestBuilder(),
		streamingTransformer: transform.NewOllamaStreamingTransformer(),
	}, nil
}

// SupportedCapabilities returns the capabilities supported by Ollama provider
func (p *Provider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{
		types.CapabilityText,
		types.CapabilityChat,
		types.CapabilityStructured,
		types.CapabilityEmbeddings,
		types.CapabilityStream,
	}
}

// Text generates a text response using Ollama's chat API
func (p *Provider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	if err := p.rejectFileReferences(request.Messages); err != nil {
		return nil, err
	}
	if request.ParallelToolCalls != nil {
		return nil, p.ValidationError("parallel_tool_calls is not supported by Ollama")
	}
	prepared, _, err := providers.PrepareMessages(request.Messages)
	if err != nil {
		return nil, err
	}
	payload := p.buildChatPayload(&request, prepared)

	url := p.GetBaseURL() + "/api/chat"

	var response chatResponse
	metadata, err := p.DoRequestWithMetadata(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	resp := p.transformTextResponse(&response)
	resp.Provider = p.Name()
	resp.ResponseMetadata = metadata
	return resp, nil
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
			case out <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// Stream generates a streaming text response using Ollama's streaming chat API
func (p *Provider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	if err := p.rejectFileReferences(request.Messages); err != nil {
		return nil, err
	}
	if request.ParallelToolCalls != nil {
		return nil, p.ValidationError("parallel_tool_calls is not supported by Ollama")
	}
	prepared, _, err := providers.PrepareMessages(request.Messages)
	if err != nil {
		return nil, err
	}
	payload := p.buildChatPayload(&request, prepared)
	payload.Stream = true

	url := p.GetBaseURL() + "/api/chat"

	body, metadata, err := p.StreamRequestWithMetadata(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	return p.streamMetadata(ctx, p.stampProvider(ctx, providerstream.ProcessNDJSON(ctx, body, p.parseStreamChunk, 100)), metadata), nil
}

// Structured generates a structured response using JSON mode
func (p *Provider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	if request.Mode == types.StructuredModeStrict {
		return nil, p.ValidationError("Ollama does not support native strict structured output")
	}
	if len(request.Tools) > 0 {
		return nil, p.ValidationError("Ollama application tools are not supported by this adapter")
	}
	// Preserve the adapter's JSON fallback when no application tools are requested.
	request.Mode = types.StructuredModeJSON
	textRequest, err := structured.Prepare(request)
	if err != nil {
		return nil, err
	}
	// JSON mode constrains syntax, not shape. Deliver the caller's schema too;
	// Prepare cloned messages, so this cannot mutate the caller's conversation.
	schemaJSON, err := json.Marshal(request.Schema)
	if err != nil {
		return nil, p.RequestError("failed to marshal schema", err)
	}
	if textRequest.SystemPrompt != "" || len(textRequest.Messages) == 0 {
		textRequest.SystemPrompt += "\n\n" + string(schemaJSON)
	} else if user, ok := textRequest.Messages[len(textRequest.Messages)-1].(*types.UserMessage); ok {
		user.Content += "\n\n" + string(schemaJSON)
	} else {
		textRequest.SystemPrompt = string(schemaJSON)
	}
	response, err := p.Text(ctx, textRequest)
	if err != nil {
		return nil, err
	}
	return structured.Convert(request, response)
}

// Embeddings generates embeddings using Ollama's embeddings API
func (p *Provider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	// Ollama embeddings API processes one input at a time
	// For multiple inputs, we process them concurrently for better performance
	if len(request.Input) == 0 {
		return nil, p.ValidationError("no input provided for embeddings")
	}

	// For small batches, process sequentially to avoid overwhelming local Ollama instance
	if len(request.Input) <= 3 {
		return p.processEmbeddingsSequentially(ctx, request)
	}

	// For larger batches, use concurrent processing
	return p.processEmbeddingsConcurrently(ctx, request)
}

// Images generates images - Ollama doesn't support image generation natively
func (p *Provider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	return nil, p.NotImplementedError("Images - Ollama does not support image generation")
}

// Audio handles both speech-to-text and text-to-speech
func (p *Provider) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	if request.Type == types.AudioRequestTypeSTT {
		return p.handleSpeechToText(ctx, request)
	}

	// Ollama doesn't support TTS
	return nil, p.NotImplementedError("TextToSpeech - Ollama does not support text-to-speech")
}

// handleSpeechToText handles speech-to-text requests
func (p *Provider) handleSpeechToText(_ context.Context, _ types.AudioRequest) (*types.AudioResponse, error) {
	// Ollama doesn't support speech-to-text directly
	return nil, p.NotImplementedError("SpeechToText - Ollama does not support speech-to-text")
}

// SpeechToText handles speech-to-text conversion - not supported by Ollama
func (p *Provider) SpeechToText(ctx context.Context, request types.SpeechToTextRequest) (*types.SpeechToTextResponse, error) {
	return nil, p.NotImplementedError("SpeechToText - Ollama does not support speech-to-text")
}

// TextToSpeech handles text-to-speech conversion - not supported by Ollama
func (p *Provider) TextToSpeech(ctx context.Context, request types.TextToSpeechRequest) (*types.TextToSpeechResponse, error) {
	return nil, p.NotImplementedError("TextToSpeech - Ollama does not support text-to-speech")
}

// GenerateImage generates an image - not supported by Ollama
func (p *Provider) GenerateImage(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
	return nil, p.NotImplementedError("GenerateImage - Ollama does not support image generation")
}
