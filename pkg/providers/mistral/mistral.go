package mistral

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/providers"
	"github.com/garyblankenship/wormhole/pkg/types"
)

const (
	defaultBaseURL = "https://api.mistral.ai/v1"
)

// Provider implements the Mistral provider
type Provider struct {
	*providers.BaseProvider
}

// New creates a new Mistral provider
func New(config types.ProviderConfig) *Provider {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}

	return &Provider{
		BaseProvider: providers.NewBaseProvider("mistral", config),
	}
}

// Text generates a text response
func (p *Provider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	payload := p.buildChatPayload(&request)

	url := p.GetBaseURL() + "/chat/completions"

	var response chatCompletionResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	return p.transformTextResponse(&response), nil
}

// Stream generates a streaming text response
func (p *Provider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	payload := p.buildChatPayload(&request)
	payload["stream"] = true

	url := p.GetBaseURL() + "/chat/completions"

	body, err := p.StreamRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	chunks := make(chan types.TextChunk, 100)

	go func() {
		defer body.Close()
		processor := utils.NewStreamProcessor(body, p.parseStreamChunk)
		processor.Process(chunks)
	}()

	return chunks, nil
}

// Structured generates a structured response
func (p *Provider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	// Convert to text request with JSON mode or function calling
	textRequest := types.TextRequest{
		BaseRequest:  request.BaseRequest,
		Messages:     request.Messages,
		SystemPrompt: request.SystemPrompt,
	}

	// Determine the best method for structured output
	if request.Mode == types.StructuredModeJSON {
		textRequest.ResponseFormat = map[string]string{"type": "json_object"}
	} else {
		// Use function calling for structured output
		tool, err := p.schemaToTool(request.Schema, request.SchemaName)
		if err != nil {
			return nil, err
		}
		textRequest.Tools = []types.Tool{*tool}
		textRequest.ToolChoice = &types.ToolChoice{
			Type:     types.ToolChoiceTypeSpecific,
			ToolName: tool.Name,
		}
	}

	response, err := p.Text(ctx, textRequest)
	if err != nil {
		return nil, err
	}

	// Extract structured data from response
	var data interface{}
	if request.Mode == types.StructuredModeJSON {
		err = json.Unmarshal([]byte(response.Text), &data)
	} else if len(response.ToolCalls) > 0 {
		argsBytes, _ := json.Marshal(response.ToolCalls[0].Arguments)
		err = json.Unmarshal(argsBytes, &data)
	} else {
		err = fmt.Errorf("no structured data in response")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse structured response: %w", err)
	}

	return &types.StructuredResponse{
		ID:      response.ID,
		Model:   response.Model,
		Data:    data,
		Usage:   response.Usage,
		Created: response.Created,
	}, nil
}

// Embeddings generates embeddings
func (p *Provider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	payload := map[string]interface{}{
		"model": request.Model,
		"input": request.Input,
	}

	// Mistral embeddings don't support dimensions parameter currently
	// if request.Dimensions != nil {
	//     payload["dimensions"] = *request.Dimensions
	// }

	url := p.GetBaseURL() + "/embeddings"

	var response embeddingsResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	return p.transformEmbeddingsResponse(&response), nil
}

// Images generates images - Mistral doesn't support image generation
func (p *Provider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	return nil, p.NotImplementedError("Images - Mistral does not support image generation")
}

// Audio handles both speech-to-text and text-to-speech
func (p *Provider) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	if request.Type == types.AudioRequestTypeSTT {
		return p.handleSpeechToText(ctx, request)
	}

	// Mistral doesn't support TTS
	return nil, p.NotImplementedError("TextToSpeech - Mistral does not support text-to-speech")
}

// handleSpeechToText handles speech-to-text requests using Mistral's API
func (p *Provider) handleSpeechToText(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	// Mistral supports speech-to-text but requires multipart form data
	// For now, return not implemented since multipart handling needs more complex implementation
	return nil, p.NotImplementedError("SpeechToText - requires multipart form data implementation")
}

// SpeechToText handles speech-to-text conversion
func (p *Provider) SpeechToText(ctx context.Context, request types.SpeechToTextRequest) (*types.SpeechToTextResponse, error) {
	// Mistral supports speech-to-text but requires multipart form data
	return nil, p.NotImplementedError("SpeechToText - requires multipart form data implementation")
}

// TextToSpeech handles text-to-speech conversion - not supported by Mistral
func (p *Provider) TextToSpeech(ctx context.Context, request types.TextToSpeechRequest) (*types.TextToSpeechResponse, error) {
	return nil, p.NotImplementedError("TextToSpeech - Mistral does not support text-to-speech")
}

// GenerateImage generates an image - not supported by Mistral
func (p *Provider) GenerateImage(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
	return nil, p.NotImplementedError("GenerateImage - Mistral does not support image generation")
}

// OCR handles OCR requests using Mistral's special OCR API
func (p *Provider) OCR(ctx context.Context, model string, documentURL string) (*types.OCRResponse, error) {
	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": "Extract all text from this document.",
					},
					{
						"type":         "document",
						"document_url": documentURL,
					},
				},
			},
		},
	}

	url := p.GetBaseURL() + "/chat/completions"

	var response ocrResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("no OCR response received")
	}

	return &types.OCRResponse{
		ID:      response.ID,
		Model:   response.Model,
		Text:    response.Choices[0].Message.Content,
		Created: response.Created,
	}, nil
}
