package prism

import (
	"context"
	"fmt"

	"github.com/prism-php/prism-go/pkg/types"
)

// AudioRequestBuilder builds audio requests (TTS and STT)
type AudioRequestBuilder struct {
	prism    *Prism
	provider string
}

// Using sets the provider to use
func (b *AudioRequestBuilder) Using(provider string) *AudioRequestBuilder {
	b.provider = provider
	return b
}

// SpeechToText creates a speech-to-text request builder
func (b *AudioRequestBuilder) SpeechToText() *SpeechToTextBuilder {
	return &SpeechToTextBuilder{
		prism:    b.prism,
		provider: b.provider,
		request:  &types.SpeechToTextRequest{},
	}
}

// TextToSpeech creates a text-to-speech request builder
func (b *AudioRequestBuilder) TextToSpeech() *TextToSpeechBuilder {
	return &TextToSpeechBuilder{
		prism:    b.prism,
		provider: b.provider,
		request:  &types.TextToSpeechRequest{},
	}
}

// SpeechToTextBuilder builds speech-to-text requests
type SpeechToTextBuilder struct {
	prism    *Prism
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
	provider, err := b.prism.getProvider(b.provider)
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

	// Ensure we have an AudioProvider
	audioProvider, ok := provider.(types.AudioProvider)
	if !ok {
		return nil, fmt.Errorf("provider %s does not support speech-to-text", provider.Name())
	}

	return audioProvider.SpeechToText(ctx, *b.request)
}

// TextToSpeechBuilder builds text-to-speech requests
type TextToSpeechBuilder struct {
	prism    *Prism
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
	provider, err := b.prism.getProvider(b.provider)
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

	// Ensure we have an AudioProvider
	audioProvider, ok := provider.(types.AudioProvider)
	if !ok {
		return nil, fmt.Errorf("provider %s does not support text-to-speech", provider.Name())
	}

	return audioProvider.TextToSpeech(ctx, *b.request)
}
