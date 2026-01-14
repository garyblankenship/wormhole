package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/providers"
	"github.com/garyblankenship/wormhole/pkg/types"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
)

// Provider implements the OpenAI provider
type Provider struct {
	*providers.BaseProvider
}

// New creates a new OpenAI provider
func New(config types.ProviderConfig) *Provider {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}

	return &Provider{
		BaseProvider: providers.NewBaseProvider("openai", config),
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

	textResponse := p.transformTextResponse(&response)

	// Validate response has content to prevent silent failures
	if textResponse.Text == "" && len(textResponse.ToolCalls) == 0 {
		return nil, fmt.Errorf("received empty response from OpenAI API: no content or tool calls returned")
	}

	return textResponse, nil
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

	return utils.ProcessStream(body, p.parseStreamChunk, 100), nil
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
	var data any
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
	payload := map[string]any{
		"model": request.Model,
		"input": request.Input,
	}

	if request.Dimensions != nil {
		payload["dimensions"] = *request.Dimensions
	}

	url := p.GetBaseURL() + "/embeddings"

	var response embeddingsResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	return p.transformEmbeddingsResponse(&response), nil
}

// Images generates images
func (p *Provider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	payload := map[string]any{
		"model":  request.Model,
		"prompt": request.Prompt,
	}

	if request.Size != "" {
		payload["size"] = request.Size
	}
	if request.Quality != "" {
		payload["quality"] = request.Quality
	}
	if request.Style != "" {
		payload["style"] = request.Style
	}
	if request.N > 0 {
		payload["n"] = request.N
	}
	if request.ResponseFormat != "" {
		payload["response_format"] = request.ResponseFormat
	}

	url := p.GetBaseURL() + "/images/generations"

	var response imageResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	return p.transformImageResponse(&response), nil
}

// Audio handles both speech-to-text and text-to-speech
func (p *Provider) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	if request.Type == types.AudioRequestTypeSTT {
		return p.handleSpeechToText(ctx, request)
	}

	// Handle TTS
	return p.handleTextToSpeech(ctx, request)
}

// handleTextToSpeech handles text-to-speech requests
func (p *Provider) handleTextToSpeech(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	payload := map[string]any{
		"model": request.Model,
		"input": request.Input,
	}

	if request.Voice != "" {
		payload["voice"] = request.Voice
	}
	if request.Speed > 0 {
		payload["speed"] = request.Speed
	}
	if request.ResponseFormat != "" {
		payload["response_format"] = request.ResponseFormat
	}

	url := p.GetBaseURL() + "/audio/speech"

	body, err := p.StreamRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	audio, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio data: %w", err)
	}

	return &types.AudioResponse{
		Model:  request.Model,
		Audio:  audio,
		Format: request.ResponseFormat,
	}, nil
}

// handleSpeechToText handles speech-to-text requests
func (p *Provider) handleSpeechToText(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	// Build multipart form data
	formData := utils.AudioFormData{
		Audio:       request.Input.([]byte),
		Filename:    "audio.wav",
		Model:       request.Model,
		Language:    request.Language,
		Prompt:      request.Prompt,
		Temperature: request.Temperature,
	}

	reader, contentType, err := utils.BuildAudioForm(formData)
	if err != nil {
		return nil, fmt.Errorf("failed to build audio form: %w", err)
	}

	// Make request to OpenAI Whisper API
	url := fmt.Sprintf("%s/audio/transcriptions", p.Config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set(types.HeaderAuthorization, "Bearer "+p.Config.APIKey)
	req.Header.Set(types.HeaderContentType, contentType)

	// Execute request
	resp, err := p.GetHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("warning: failed to close response body: %v", err)
		}
	}()

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.Errorf("read response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var sttResponse struct {
		Text     string  `json:"text"`
		Language string  `json:"language,omitempty"`
		Duration float64 `json:"duration,omitempty"`
	}

	if err := json.Unmarshal(body, &sttResponse); err != nil {
		return nil, types.Errorf("parse response", err)
	}

	return &types.AudioResponse{
		Text:   sttResponse.Text,
		Format: "text",
	}, nil
}

// Temporarily disabled until request types are defined
// These methods will be automatically provided by embedded BaseProvider with NotImplementedError
