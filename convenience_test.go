package wormhole

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestImageBuilderConfigurationAndValidation(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	builder := client.Image().
		Using("openai").
		BaseURL("https://example.test/v1").
		Model("gpt-image-1").
		Prompt("draw a cube").
		Size("1024x1024").
		Quality("high").
		Style("natural").
		N(2).
		ResponseFormat("b64_json")

	if builder.getProvider() != "openai" || builder.getBaseURL() != "https://example.test/v1" {
		t.Fatalf("image builder routing = (%q, %q)", builder.getProvider(), builder.getBaseURL())
	}
	if builder.request.Model != "gpt-image-1" || builder.request.Prompt != "draw a cube" {
		t.Fatalf("image request = %#v", builder.request)
	}
	if builder.request.Size != "1024x1024" || builder.request.Quality != "high" || builder.request.Style != "natural" {
		t.Fatalf("image options = %#v", builder.request)
	}
	if builder.request.N != 2 || builder.request.ResponseFormat != "b64_json" {
		t.Fatalf("image output options = %#v", builder.request)
	}

	ctx := context.Background()
	if _, err := client.Image().Model("gpt-image-1").Generate(ctx); err == nil {
		t.Fatal("Generate without prompt returned nil error")
	}
	if _, err := client.Image().Prompt("draw").Generate(ctx); err == nil {
		t.Fatal("Generate without model returned nil error")
	}

	cloned := cloneImageRequest(builder.request)
	cloned.ProviderOptions = map[string]any{"trace": true}
	if builder.request.ProviderOptions != nil {
		t.Fatal("cloneImageRequest mutation changed original ProviderOptions")
	}
}

func TestAudioBuilderConfigurationAndValidation(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))
	audio := client.Audio().Using("openai")
	if audio.provider != "openai" {
		t.Fatalf("audio provider = %q, want openai", audio.provider)
	}
	if got := resolveAudioProvider("", client); got != "openai" {
		t.Fatalf("resolveAudioProvider default = %q, want openai", got)
	}

	stt := audio.SpeechToText().
		Model("whisper-1").
		Audio([]byte("abc"), "wav").
		Language("en").
		Prompt("names").
		Temperature(0.1)
	if stt.request.Model != "whisper-1" || stt.request.AudioFormat != "wav" || stt.request.Language != "en" {
		t.Fatalf("stt request = %#v", stt.request)
	}
	if *stt.request.Temperature != 0.1 {
		t.Fatalf("stt temperature = %v, want 0.1", *stt.request.Temperature)
	}

	tts := audio.TextToSpeech().
		Model("tts-1").
		Input("hello").
		Voice("alloy").
		Speed(1.2).
		ResponseFormat("mp3")
	if tts.request.Model != "tts-1" || tts.request.Input != "hello" || tts.request.Voice != "alloy" {
		t.Fatalf("tts request = %#v", tts.request)
	}
	if tts.request.Speed != 1.2 || tts.request.ResponseFormat != "mp3" {
		t.Fatalf("tts output options = %#v", tts.request)
	}

	ctx := context.Background()
	if _, err := client.Audio().SpeechToText().Model("whisper-1").Transcribe(ctx); err == nil {
		t.Fatal("Transcribe without audio returned nil error")
	}
	if _, err := client.Audio().SpeechToText().Audio([]byte("abc"), "wav").Transcribe(ctx); err == nil {
		t.Fatal("Transcribe without model returned nil error")
	}
	if _, err := client.Audio().TextToSpeech().Model("tts-1").Voice("alloy").Generate(ctx); err == nil {
		t.Fatal("TTS without input returned nil error")
	}
	if _, err := client.Audio().TextToSpeech().Input("hello").Voice("alloy").Generate(ctx); err == nil {
		t.Fatal("TTS without model returned nil error")
	}
	if _, err := client.Audio().TextToSpeech().Model("tts-1").Input("hello").Generate(ctx); err == nil {
		t.Fatal("TTS without voice returned nil error")
	}
}

func TestAudioResponseConversions(t *testing.T) {
	t.Parallel()

	resp := types.AudioResponse{
		ID:       "id",
		Model:    "model",
		Audio:    []byte("audio"),
		Text:     "text",
		Format:   "mp3",
		Metadata: map[string]any{"k": "v"},
	}

	stt := audioResponseToSTT(resp)
	if stt.ID != "id" || stt.Text != "text" || stt.Metadata["k"] != "v" {
		t.Fatalf("audioResponseToSTT = %#v", stt)
	}
	tts := audioResponseToTTS(resp)
	if tts.ID != "id" || string(tts.Audio) != "audio" || tts.Format != "mp3" {
		t.Fatalf("audioResponseToTTS = %#v", tts)
	}
}

func TestWormholeConvenienceConstructorsAndLifecycle(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("openai"), WithOpenAI("test-key"), WithModelValidation(false), WithDiscovery(false))

	if client.Image() == nil {
		t.Fatal("Image() returned nil")
	}
	if client.Audio() == nil {
		t.Fatal("Audio() returned nil")
	}
	agent := client.Agent()
	if agent == nil || agent.maxSteps != 10 || agent.tools == nil {
		t.Fatalf("Agent() = %#v", agent)
	}
	if client.IsShuttingDown() {
		t.Fatal("new client is shutting down")
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !client.IsShuttingDown() {
		t.Fatal("client not marked shutting down after Close")
	}
}

func TestToolRegistryListNamesAndLimiterCapacity(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry()
	registry.Register("weather", types.NewToolDefinition(*types.NewTool("weather", "Weather", nil), nil))
	registry.Register("lookup", types.NewToolDefinition(*types.NewTool("lookup", "Lookup", nil), nil))

	names := registry.ListNames()
	slices.Sort(names)
	if !slices.Equal(names, []string{"lookup", "weather"}) {
		t.Fatalf("ListNames() = %#v", names)
	}

	limiter := NewConcurrencyLimiter(3)
	if limiter.Capacity() != 3 {
		t.Fatalf("Capacity() = %d, want 3", limiter.Capacity())
	}
	unlimited := NewConcurrencyLimiter(0)
	if unlimited.Capacity() != 1024 {
		t.Fatalf("unlimited Capacity() = %d, want 1024", unlimited.Capacity())
	}
}

func TestRetryExecutor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	executor := NewRetryExecutor(-1)
	calls := 0
	if err := executor.ExecuteWithRetry(ctx, func(ctx context.Context) error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("ExecuteWithRetry success returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("success calls = %d, want 1", calls)
	}

	wantErr := errors.New("fail")
	err := executor.ExecuteWithRetry(ctx, func(ctx context.Context) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExecuteWithRetry error = %v, want %v", err, wantErr)
	}
}

func TestRetryExecutorDoesNotRetryNonRetryableError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	executor := NewRetryExecutor(5)
	calls := 0
	sideEffectErr := errors.New("email already sent")

	err := executor.ExecuteWithRetry(ctx, func(ctx context.Context) error {
		calls++
		return NonRetryableToolError(sideEffectErr)
	})

	if !errors.Is(err, sideEffectErr) {
		t.Fatalf("ExecuteWithRetry error = %v, want wrapped %v", err, sideEffectErr)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1: a non-retryable error must not be retried", calls)
	}
}

func TestRetryExecutorCustomRetryableFunc(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	executor := NewRetryExecutor(5).WithRetryableFunc(func(err error) bool {
		return false // never retry, regardless of error type
	})
	calls := 0
	wantErr := errors.New("plain error")

	err := executor.ExecuteWithRetry(ctx, func(ctx context.Context) error {
		calls++
		return wantErr
	})

	if !errors.Is(err, wantErr) {
		t.Fatalf("ExecuteWithRetry error = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1: custom RetryableFunc returning false must stop retries", calls)
	}
}
