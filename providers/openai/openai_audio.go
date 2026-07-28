package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/garyblankenship/wormhole/v2/types"
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
	formData := audioFormData{
		audio:       audio,
		filename:    "audio.wav",
		model:       request.Model,
		language:    request.Language,
		prompt:      request.Prompt,
		temperature: request.Temperature,
	}

	formBody, contentType, err := buildAudioForm(formData)
	if err != nil {
		return nil, p.RequestError("failed to build audio form", err)
	}

	url := p.GetBaseURL() + "/audio/transcriptions"
	resp, err := p.DoRawRequest(ctx, http.MethodPost, url, contentType, formBody)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Close()
	}()

	// Parse response
	body, err := readLimited(resp, maxSpeechToTextJSONBytes)
	if err != nil {
		return nil, types.Errorf("read response", err)
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
