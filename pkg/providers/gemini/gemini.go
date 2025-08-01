package gemini

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/prism-php/prism-go/pkg/providers"
	"github.com/prism-php/prism-go/pkg/types"
)

const (
	defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
)

// Gemini provider implementation
type Gemini struct {
	*providers.BaseProvider
	apiKey string
}

// New creates a new Gemini provider
func New(apiKey string, config types.ProviderConfig) *Gemini {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}

	// Gemini uses API key in URL, not in Authorization header
	config.APIKey = ""

	return &Gemini{
		BaseProvider: providers.NewBaseProvider("gemini", config),
		apiKey:       apiKey,
	}
}

// Name returns the provider name
func (g *Gemini) Name() string {
	return "gemini"
}

// Text generates text using Gemini models
func (g *Gemini) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	payload, err := g.buildTextPayload(request)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		g.GetBaseURL(),
		request.Model,
		g.apiKey,
	)

	var response geminiTextResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	return g.transformTextResponse(&response)
}

// Stream generates streaming text using Gemini models
func (g *Gemini) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	payload, err := g.buildTextPayload(request)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/models/%s:streamGenerateContent?key=%s",
		g.GetBaseURL(),
		request.Model,
		g.apiKey,
	)

	stream, err := g.StreamRequest(ctx, "POST", endpoint, payload)
	if err != nil {
		return nil, err
	}

	return g.handleStream(stream), nil
}

// Structured generates structured output using Gemini models
func (g *Gemini) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	payload, err := g.buildStructuredPayload(request)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		g.GetBaseURL(),
		request.Model,
		g.apiKey,
	)

	var response geminiTextResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	return g.transformStructuredResponse(&response, request.Schema)
}

// Embeddings generates embeddings using Gemini models
func (g *Gemini) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	if !strings.Contains(request.Model, "embedding") {
		return nil, errors.New("model must be an embedding model")
	}

	payload := g.buildEmbeddingsPayload(request)

	endpoint := fmt.Sprintf("%s/models/%s:batchEmbedContents?key=%s",
		g.GetBaseURL(),
		request.Model,
		g.apiKey,
	)

	var response geminiEmbeddingsResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	return g.transformEmbeddingsResponse(&response)
}

// Audio is not supported by Gemini
func (g *Gemini) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	return nil, g.NotImplementedError("Audio")
}

// Images is not supported by Gemini
func (g *Gemini) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	return nil, g.NotImplementedError("images")
}

// buildTextPayload builds the request payload for text generation
func (g *Gemini) buildTextPayload(request types.TextRequest) (map[string]interface{}, error) {
	contents, err := g.transformMessages(request.Messages)
	if err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"contents": contents,
	}

	if request.SystemPrompt != "" {
		payload["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": request.SystemPrompt},
			},
		}
	}

	// Add generation config
	generationConfig := map[string]interface{}{}
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		generationConfig["maxOutputTokens"] = *request.MaxTokens
	}
	if request.Temperature != nil {
		generationConfig["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		generationConfig["topP"] = *request.TopP
	}
	if len(request.Stop) > 0 {
		generationConfig["stopSequences"] = request.Stop
	}

	if len(generationConfig) > 0 {
		payload["generationConfig"] = generationConfig
	}

	// Add tools if provided
	if len(request.Tools) > 0 {
		tools, err := g.transformTools(request.Tools)
		if err != nil {
			return nil, err
		}
		payload["tools"] = tools

		// Add tool config if specified
		if request.ToolChoice != nil {
			payload["toolConfig"] = g.transformToolChoice(request.ToolChoice)
		}
	}

	return payload, nil
}

// buildStructuredPayload builds the request payload for structured generation
func (g *Gemini) buildStructuredPayload(request types.StructuredRequest) (map[string]interface{}, error) {
	// For Gemini, we use response schema in generation config
	textRequest := types.TextRequest{
		BaseRequest:  request.BaseRequest,
		Messages:     request.Messages,
		SystemPrompt: request.SystemPrompt,
	}

	payload, err := g.buildTextPayload(textRequest)
	if err != nil {
		return nil, err
	}

	// Add response schema to generation config
	if generationConfig, ok := payload["generationConfig"].(map[string]interface{}); ok {
		generationConfig["responseMimeType"] = "application/json"
		generationConfig["responseSchema"] = g.transformSchema(request.Schema)
	} else {
		payload["generationConfig"] = map[string]interface{}{
			"responseMimeType": "application/json",
			"responseSchema":   g.transformSchema(request.Schema),
		}
	}

	return payload, nil
}

// buildEmbeddingsPayload builds the request payload for embeddings
func (g *Gemini) buildEmbeddingsPayload(request types.EmbeddingsRequest) map[string]interface{} {
	requests := make([]map[string]interface{}, len(request.Input))

	for i, input := range request.Input {
		requests[i] = map[string]interface{}{
			"content": map[string]interface{}{
				"parts": []map[string]interface{}{
					{"text": input},
				},
			},
		}

		// Add task type if specified
		if request.ProviderOptions != nil {
			if taskType, ok := request.ProviderOptions["taskType"].(string); ok {
				requests[i]["taskType"] = taskType
			}
			if title, ok := request.ProviderOptions["title"].(string); ok {
				requests[i]["title"] = title
			}
		}
	}

	return map[string]interface{}{
		"requests": requests,
	}
}
