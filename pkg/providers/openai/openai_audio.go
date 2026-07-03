package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/types"
)

const (
	maxTextToSpeechAudioBytes = 64 << 20
	maxSpeechToTextJSONBytes  = 1 << 20
)

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
	defer func() {
		_ = body.Close()
	}()

	audio, err := readLimited(body, maxTextToSpeechAudioBytes)
	if err != nil {
		return nil, p.RequestError("failed to read audio data", err)
	}

	return &types.AudioResponse{
		Model:  request.Model,
		Audio:  audio,
		Format: request.ResponseFormat,
	}, nil
}

// handleSpeechToText handles speech-to-text requests
func (p *Provider) handleSpeechToText(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	audio, ok := request.Input.([]byte)
	if !ok || len(audio) == 0 {
		return nil, p.ValidationError("speech-to-text input must be non-empty []byte audio")
	}

	// Build multipart form data
	formData := utils.AudioFormData{
		Audio:       audio,
		Filename:    "audio.wav",
		Model:       request.Model,
		Language:    request.Language,
		Prompt:      request.Prompt,
		Temperature: request.Temperature,
	}

	reader, contentType, err := utils.BuildAudioForm(formData)
	if err != nil {
		return nil, p.RequestError("failed to build audio form", err)
	}

	// Make request to OpenAI Whisper API
	url := fmt.Sprintf("%s/audio/transcriptions", p.Config.BaseURL)
	reqCtx, cancel := p.RequestContext(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", url, reader)
	if err != nil {
		return nil, p.RequestError("failed to create request", err)
	}

	// Set headers
	req.Header.Set(types.HeaderAuthorization, "Bearer "+p.Config.APIKey)
	req.Header.Set(types.HeaderContentType, contentType)

	// Execute request
	resp, err := p.GetHTTPClient().Do(req)
	if err != nil {
		return nil, p.WrapError(types.ErrorCodeNetwork, "request failed", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("failed to close response body", "error", err)
		}
	}()

	// Parse response
	body, err := readLimited(resp.Body, maxSpeechToTextJSONBytes)
	if err != nil {
		return nil, types.Errorf("read response", err)
	}

	if resp.StatusCode != http.StatusOK {
		err := types.HTTPStatusToError(resp.StatusCode, string(body))
		err.Provider = p.Name()
		return nil, err
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

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response body exceeded %d bytes", limit)
	}
	return data, nil
}
