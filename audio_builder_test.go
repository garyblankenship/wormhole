package wormhole_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2"
	"github.com/garyblankenship/wormhole/v2/types"
	mocktesting "github.com/garyblankenship/wormhole/v2/wormholetest"
)

func TestSpeechToTextBuilder(t *testing.T) {
	t.Parallel()

	mockProvider := mocktesting.NewMockProvider("mock")
	client := wormhole.New(
		wormhole.WithDefaultProvider("mock"),
		wormhole.WithCustomProvider("mock", mocktesting.MockProviderFactory(mockProvider)),
		wormhole.WithProviderConfig("mock", types.ProviderConfig{}),
	)

	ctx := context.Background()

	t.Run("validation error - missing audio", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Audio().
			SpeechToText().
			Model("whisper-1").
			Transcribe(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "no audio data provided")
	})

	t.Run("validation error - missing model", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Audio().
			SpeechToText().
			Audio([]byte("audio-bytes"), "mp3").
			Transcribe(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "no model specified")
	})

	t.Run("successful transcription", func(t *testing.T) {
		t.Parallel()
		temp := float32(0.2)
		resp, err := client.Audio().
			Using("mock").
			SpeechToText().
			Model("whisper-1").
			Audio([]byte("test audio content"), "wav").
			Language("en").
			Prompt("transcribe clearly").
			Temperature(temp).
			Transcribe(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "whisper-1", resp.Model)
		assert.Equal(t, "Mock transcribed text", resp.Text)
		assert.Equal(t, "mock-stt", resp.ID)
	})

	t.Run("provider error", func(t *testing.T) {
		t.Parallel()
		errProvider := mocktesting.NewMockProvider("err-provider").WithError("stt provider failure")
		errClient := wormhole.New(
			wormhole.WithDefaultProvider("err-provider"),
			wormhole.WithCustomProvider("err-provider", mocktesting.MockProviderFactory(errProvider)),
			wormhole.WithProviderConfig("err-provider", types.ProviderConfig{}),
		)

		resp, err := errClient.Audio().
			SpeechToText().
			Model("whisper-1").
			Audio([]byte("data"), "mp3").
			Transcribe(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "stt provider failure")
	})
}

func TestTextToSpeechBuilder(t *testing.T) {
	t.Parallel()

	mockProvider := mocktesting.NewMockProvider("mock")
	client := wormhole.New(
		wormhole.WithDefaultProvider("mock"),
		wormhole.WithCustomProvider("mock", mocktesting.MockProviderFactory(mockProvider)),
		wormhole.WithProviderConfig("mock", types.ProviderConfig{}),
	)

	ctx := context.Background()

	t.Run("validation error - missing input", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Audio().
			TextToSpeech().
			Model("tts-1").
			Voice("alloy").
			Generate(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "no input text provided")
	})

	t.Run("validation error - missing model", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Audio().
			TextToSpeech().
			Input("Hello world").
			Voice("alloy").
			Generate(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "no model specified")
	})

	t.Run("validation error - missing voice", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Audio().
			TextToSpeech().
			Model("tts-1").
			Input("Hello world").
			Generate(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "no voice specified")
	})

	t.Run("successful generation", func(t *testing.T) {
		t.Parallel()
		resp, err := client.Audio().
			Using("mock").
			TextToSpeech().
			Model("tts-1").
			Input("Hello world").
			Voice("alloy").
			Speed(1.2).
			ResponseFormat("mp3").
			Generate(ctx)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "tts-1", resp.Model)
		assert.Equal(t, []byte("mock audio data"), resp.Audio)
		assert.Equal(t, "mp3", resp.Format)
		assert.Equal(t, "mock-audio", resp.ID)
	})

	t.Run("provider error", func(t *testing.T) {
		t.Parallel()
		errProvider := mocktesting.NewMockProvider("err-provider").WithError("tts failed")
		errClient := wormhole.New(
			wormhole.WithDefaultProvider("err-provider"),
			wormhole.WithCustomProvider("err-provider", mocktesting.MockProviderFactory(errProvider)),
			wormhole.WithProviderConfig("err-provider", types.ProviderConfig{}),
		)

		resp, err := errClient.Audio().
			TextToSpeech().
			Model("tts-1").
			Input("Hello").
			Voice("alloy").
			Generate(ctx)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "tts failed")
	})
}
