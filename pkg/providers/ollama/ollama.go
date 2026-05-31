package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/providers"
	transform "github.com/garyblankenship/wormhole/pkg/providers/transform"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// No default base URLs - Ollama must be configured with explicit URL

// Provider implements the Ollama provider
type Provider struct {
	*providers.BaseProvider
	requestBuilder       *providers.RequestBuilder
	responseTransform    *transform.ResponseTransform
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
		responseTransform:    transform.NewResponseTransform(),
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
	payload := p.buildChatPayload(&request)

	url := p.GetBaseURL() + "/api/chat"

	var response chatResponse
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
func (p *Provider) stampProvider(in <-chan types.TextChunk) <-chan types.TextChunk {
	out := make(chan types.TextChunk)
	go func() {
		defer close(out)
		for chunk := range in {
			if chunk.IsDone() {
				chunk.Provider = p.Name()
			}
			out <- chunk
		}
	}()
	return out
}

// Stream generates a streaming text response using Ollama's streaming chat API
func (p *Provider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	payload := p.buildChatPayload(&request)
	payload.Stream = true

	url := p.GetBaseURL() + "/api/chat"

	body, err := p.StreamRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	return p.stampProvider(utils.ProcessStream(body, p.parseStreamChunk, 100)), nil
}

// Structured generates a structured response using JSON mode
func (p *Provider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	// Convert to text request with JSON mode
	textRequest := types.TextRequest{
		BaseRequest:  request.BaseRequest,
		Messages:     request.Messages,
		SystemPrompt: request.SystemPrompt,
	}

	// Use JSON format for structured output
	if request.Mode == types.StructuredModeJSON {
		textRequest.ResponseFormat = map[string]string{"type": "json_object"}
	} else {
		// Ollama doesn't support function calling, fallback to JSON mode
		textRequest.ResponseFormat = map[string]string{"type": "json_object"}

		// Add schema instruction to system prompt or last user message
		schemaBytes, err := json.Marshal(request.Schema)
		if err != nil {
			return nil, p.RequestError("failed to marshal schema", err)
		}

		schemaInstruction := fmt.Sprintf("Please respond with valid JSON that conforms to this schema: %s", string(schemaBytes))

		if textRequest.SystemPrompt != "" {
			textRequest.SystemPrompt += "\n\n" + schemaInstruction
		} else {
			// Add to last user message
			if len(textRequest.Messages) > 0 {
				lastMsg := textRequest.Messages[len(textRequest.Messages)-1]
				if userMsg, ok := lastMsg.(*types.UserMessage); ok {
					userMsg.Content = userMsg.Content + "\n\n" + schemaInstruction
				}
			}
		}
	}

	response, err := p.Text(ctx, textRequest)
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	var data any
	err = json.Unmarshal([]byte(response.Text), &data)
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
