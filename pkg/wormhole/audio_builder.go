package wormhole

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// AudioRequestBuilder builds audio requests (TTS and STT)
type AudioRequestBuilder struct {
	wormhole *Wormhole
	provider string
}

// Using sets the provider to use
func (b *AudioRequestBuilder) Using(provider string) *AudioRequestBuilder {
	b.provider = provider
	return b
}

// Provider sets the provider to use (alias for Using)
func (b *AudioRequestBuilder) Provider(provider string) *AudioRequestBuilder {
	b.provider = provider
	return b
}

// SpeechToText creates a speech-to-text request builder
func (b *AudioRequestBuilder) SpeechToText() *SpeechToTextBuilder {
	return &SpeechToTextBuilder{
		wormhole: b.wormhole,
		provider: b.provider,
		request:  &types.SpeechToTextRequest{},
	}
}

// TextToSpeech creates a text-to-speech request builder
func (b *AudioRequestBuilder) TextToSpeech() *TextToSpeechBuilder {
	return &TextToSpeechBuilder{
		wormhole: b.wormhole,
		provider: b.provider,
		request:  &types.TextToSpeechRequest{},
	}
}

// SpeechToTextBuilder builds speech-to-text requests
type SpeechToTextBuilder struct {
	wormhole *Wormhole
	provider string
	request  *types.SpeechToTextRequest
}

// Model sets the model to use
func (b *SpeechToTextBuilder) Model(model string) *SpeechToTextBuilder {
	b.request.Model = model
	return b
}

// Audio sets the audio data
func (b *SpeechToTextBuilder) Audio(audio []byte, format string) *SpeechToTextBuilder {
	b.request.Audio = audio
	b.request.AudioFormat = format
	return b
}

// Language sets the language of the audio
func (b *SpeechToTextBuilder) Language(lang string) *SpeechToTextBuilder {
	b.request.Language = lang
	return b
}

// Prompt sets an optional prompt to guide the transcription
func (b *SpeechToTextBuilder) Prompt(prompt string) *SpeechToTextBuilder {
	b.request.Prompt = prompt
	return b
}

// Temperature sets the temperature for transcription
func (b *SpeechToTextBuilder) Temperature(temp float32) *SpeechToTextBuilder {
	b.request.Temperature = &temp
	return b
}

// Transcribe executes the request and returns transcribed text
func (b *SpeechToTextBuilder) Transcribe(ctx context.Context) (*types.SpeechToTextResponse, error) {
	provider, err := b.wormhole.getProvider(b.provider)
	if err != nil {
		return nil, err
	}

	// Validate request
	if len(b.request.Audio) == 0 {
		return nil, fmt.Errorf("no audio data provided")
	}
	if b.request.Model == "" {
		return nil, fmt.Errorf("no model specified")
	}

	// Convert to AudioRequest
	audioRequest := types.AudioRequest{
		Type:        types.AudioRequestTypeSTT,
		Model:       b.request.Model,
		Input:       b.request.Audio,
		Language:    b.request.Language,
		Prompt:      b.request.Prompt,
		Temperature: b.request.Temperature,
	}

	// Apply type-safe middleware chain if configured
	if b.wormhole.providerMiddleware != nil {
		handler := b.wormhole.providerMiddleware.ApplyAudio(provider.Audio)
		audioResp, err := handler(ctx, audioRequest)
		if err != nil {
			return nil, err
		}
		// Convert back to SpeechToTextResponse
		return &types.SpeechToTextResponse{
			ID:       audioResp.ID,
			Model:    audioResp.Model,
			Text:     audioResp.Text,
			Created:  audioResp.Created,
			Metadata: audioResp.Metadata,
		}, nil
	}

	// Fallback to legacy middleware if configured
	if b.wormhole.middlewareChain != nil {
		handler := b.wormhole.middlewareChain.Apply(func(ctx context.Context, req any) (any, error) {
			audioReq := req.(*types.AudioRequest)
			return provider.Audio(ctx, *audioReq)
		})
		resp, err := handler(ctx, &audioRequest)
		if err != nil {
			return nil, err
		}
		audioResp := resp.(*types.AudioResponse)
		return &types.SpeechToTextResponse{
			ID:       audioResp.ID,
			Model:    audioResp.Model,
			Text:     audioResp.Text,
			Created:  audioResp.Created,
			Metadata: audioResp.Metadata,
		}, nil
	}

	// Direct call
	audioResp, err := provider.Audio(ctx, audioRequest)
	if err != nil {
		return nil, err
	}
	return &types.SpeechToTextResponse{
		ID:       audioResp.ID,
		Model:    audioResp.Model,
		Text:     audioResp.Text,
		Created:  audioResp.Created,
		Metadata: audioResp.Metadata,
	}, nil
}

// TextToSpeechBuilder builds text-to-speech requests
type TextToSpeechBuilder struct {
	wormhole *Wormhole
	provider string
	request  *types.TextToSpeechRequest
}

// Model sets the model to use
func (b *TextToSpeechBuilder) Model(model string) *TextToSpeechBuilder {
	b.request.Model = model
	return b
}

// Input sets the text to convert to speech
func (b *TextToSpeechBuilder) Input(text string) *TextToSpeechBuilder {
	b.request.Input = text
	return b
}

// Voice sets the voice to use
func (b *TextToSpeechBuilder) Voice(voice string) *TextToSpeechBuilder {
	b.request.Voice = voice
	return b
}

// Speed sets the speech speed
func (b *TextToSpeechBuilder) Speed(speed float32) *TextToSpeechBuilder {
	b.request.Speed = speed
	return b
}

// ResponseFormat sets the audio response format
func (b *TextToSpeechBuilder) ResponseFormat(format string) *TextToSpeechBuilder {
	b.request.ResponseFormat = format
	return b
}

// Generate executes the request and returns audio
func (b *TextToSpeechBuilder) Generate(ctx context.Context) (*types.TextToSpeechResponse, error) {
	provider, err := b.wormhole.getProvider(b.provider)
	if err != nil {
		return nil, err
	}

	// Validate request
	if b.request.Input == "" {
		return nil, fmt.Errorf("no input text provided")
	}
	if b.request.Model == "" {
		return nil, fmt.Errorf("no model specified")
	}
	if b.request.Voice == "" {
		return nil, fmt.Errorf("no voice specified")
	}

	// Convert to AudioRequest
	audioRequest := types.AudioRequest{
		Type:            types.AudioRequestTypeTTS,
		Model:           b.request.Model,
		Input:           b.request.Input,
		Voice:           b.request.Voice,
		Speed:           b.request.Speed,
		ResponseFormat:  b.request.ResponseFormat,
		ProviderOptions: b.request.ProviderOptions,
	}

	// Apply type-safe middleware chain if configured
	if b.wormhole.providerMiddleware != nil {
		handler := b.wormhole.providerMiddleware.ApplyAudio(provider.Audio)
		audioResp, err := handler(ctx, audioRequest)
		if err != nil {
			return nil, err
		}
		// Convert back to TextToSpeechResponse
		return &types.TextToSpeechResponse{
			ID:       audioResp.ID,
			Model:    audioResp.Model,
			Audio:    audioResp.Audio,
			Format:   audioResp.Format,
			Created:  audioResp.Created,
			Metadata: audioResp.Metadata,
		}, nil
	}

	// Fallback to legacy middleware if configured
	if b.wormhole.middlewareChain != nil {
		handler := b.wormhole.middlewareChain.Apply(func(ctx context.Context, req any) (any, error) {
			audioReq := req.(*types.AudioRequest)
			return provider.Audio(ctx, *audioReq)
		})
		resp, err := handler(ctx, &audioRequest)
		if err != nil {
			return nil, err
		}
		audioResp := resp.(*types.AudioResponse)
		return &types.TextToSpeechResponse{
			ID:       audioResp.ID,
			Model:    audioResp.Model,
			Audio:    audioResp.Audio,
			Format:   audioResp.Format,
			Created:  audioResp.Created,
			Metadata: audioResp.Metadata,
		}, nil
	}

	// Direct call
	audioResp, err := provider.Audio(ctx, audioRequest)
	if err != nil {
		return nil, err
	}
	return &types.TextToSpeechResponse{
		ID:       audioResp.ID,
		Model:    audioResp.Model,
		Audio:    audioResp.Audio,
		Format:   audioResp.Format,
		Created:  audioResp.Created,
		Metadata: audioResp.Metadata,
	}, nil
}
