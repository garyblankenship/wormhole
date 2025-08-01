package groq

import (
	"context"
	"errors"
	"fmt"

	"github.com/prism-php/prism-go/pkg/providers"
	"github.com/prism-php/prism-go/pkg/types"
)

const (
	defaultBaseURL = "https://api.groq.com/openai/v1"
)

// Groq provider implementation
type Groq struct {
	*providers.BaseProvider
}

// New creates a new Groq provider
func New(apiKey string, config types.ProviderConfig) *Groq {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	config.APIKey = apiKey

	return &Groq{
		BaseProvider: providers.NewBaseProvider("groq", config),
	}
}

// Name returns the provider name
func (g *Groq) Name() string {
	return "groq"
}

// Text generates text using Groq models
func (g *Groq) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	payload := g.buildTextPayload(request)

	endpoint := fmt.Sprintf("%s/chat/completions", g.GetBaseURL())

	var response groqTextResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	return g.transformTextResponse(&response)
}

// Stream generates streaming text using Groq models
func (g *Groq) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	payload := g.buildTextPayload(request)
	payload["stream"] = true

	endpoint := fmt.Sprintf("%s/chat/completions", g.GetBaseURL())

	stream, err := g.StreamRequest(ctx, "POST", endpoint, payload)
	if err != nil {
		return nil, err
	}

	return g.handleStream(stream), nil
}

// Structured generates structured output using Groq models
func (g *Groq) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	// Groq uses OpenAI-compatible API with response_format
	payload := g.buildTextPayload(types.TextRequest{
		BaseRequest:  request.BaseRequest,
		Messages:     request.Messages,
		SystemPrompt: request.SystemPrompt,
	})

	// Add response format for JSON mode
	payload["response_format"] = map[string]interface{}{
		"type": "json_object",
	}

	endpoint := fmt.Sprintf("%s/chat/completions", g.GetBaseURL())

	var response groqTextResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	return g.transformStructuredResponse(&response, request.Schema)
}

// Embeddings is not supported by Groq
func (g *Groq) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	return nil, g.NotImplementedError("embeddings")
}

// Audio handles both text-to-speech and speech-to-text
func (g *Groq) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	if request.Type == types.AudioRequestTypeTTS {
		// Groq doesn't support TTS
		return nil, errors.New("text-to-speech is not supported by Groq")
	}

	// Handle speech-to-text
	return g.handleSpeechToText(ctx, request)
}

// Images is not supported by Groq
func (g *Groq) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	return nil, g.NotImplementedError("images")
}

// buildTextPayload builds the request payload for text generation
func (g *Groq) buildTextPayload(request types.TextRequest) map[string]interface{} {
	messages := g.transformMessages(request.Messages)

	// Add system prompt if provided
	if request.SystemPrompt != "" {
		systemMsg := map[string]interface{}{
			"role":    "system",
			"content": request.SystemPrompt,
		}
		messages = append([]map[string]interface{}{systemMsg}, messages...)
	}

	payload := map[string]interface{}{
		"model":    request.Model,
		"messages": messages,
	}

	// Add optional parameters
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		payload["max_tokens"] = *request.MaxTokens
	}
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if len(request.Stop) > 0 {
		payload["stop"] = request.Stop
	}

	// Add tools if provided
	if len(request.Tools) > 0 {
		tools := g.transformTools(request.Tools)
		payload["tools"] = tools

		// Add tool choice if specified
		if request.ToolChoice != nil {
			payload["tool_choice"] = g.transformToolChoice(request.ToolChoice)
		}
	}

	// Add provider-specific options
	if request.ProviderOptions != nil {
		for k, v := range request.ProviderOptions {
			payload[k] = v
		}
	}

	return payload
}

// handleSpeechToText handles speech-to-text requests
func (g *Groq) handleSpeechToText(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	if request.Input == nil {
		return nil, errors.New("audio input is required for speech-to-text")
	}

	// Groq uses OpenAI-compatible transcription API but requires multipart form data
	// For now, return not implemented since multipart handling needs more complex implementation
	return nil, g.NotImplementedError("SpeechToText - requires multipart form data implementation")
}
