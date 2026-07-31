package middleware

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func newTestLogger(buf *bytes.Buffer) types.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLoggingHelpers(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	config := DefaultLoggingConfig(newTestLogger(&buf))
	if len(config.RedactKeys) == 0 || !config.LogRequests || !config.LogResponses || !config.LogTiming || !config.LogErrors {
		t.Fatalf("DefaultLoggingConfig = %#v", config)
	}
	logRequestDetails(config, testTextRequest(strings.Repeat("x", 120)))
	logRequestDetails(config, types.StructuredRequest{BaseRequest: types.BaseRequest{Model: "gpt"}, Schema: map[string]any{"type": "object"}})
	logRequestDetails(config, types.EmbeddingsRequest{Model: "embed", Input: []string{"a"}})
	logRequestDetails(config, types.AudioRequest{Type: types.AudioRequestTypeTTS, Model: "tts", Voice: "alloy"})
	logRequestDetails(config, types.ImageRequest{Model: "image", Prompt: strings.Repeat("p", 120), Size: "1024x1024", Quality: "high", N: 1})
	logRequestDetails(config, struct {
		APIKey string `json:"api_key"`
	}{APIKey: "secret"})
	logResponseDetails(config, &types.TextResponse{Model: "gpt", Text: strings.Repeat("x", 220), FinishReason: types.FinishReasonStop, Usage: &types.Usage{TotalTokens: 3}, ToolCalls: []types.ToolCall{{Name: "lookup"}}}, time.Millisecond)
	logResponseDetails(config, &types.StructuredResponse{Model: "gpt", Data: map[string]any{"ok": true}}, time.Millisecond)
	logResponseDetails(config, &types.EmbeddingsResponse{Model: "embed", Embeddings: []types.Embedding{{Embedding: []float64{1, 2}}}}, time.Millisecond)
	logResponseDetails(config, &types.AudioResponse{Model: "audio", Text: strings.Repeat("a", 120), Audio: []byte("abc"), Created: time.Now()}, time.Millisecond)
	logResponseDetails(config, &types.ImageResponse{Model: "image", Images: []types.GeneratedImage{{URL: "https://example.test"}}}, time.Millisecond)
	logResponseDetails(config, "plain", time.Millisecond)
	logError(context.Background(), config, errors.New("plain"), time.Millisecond)
	if buf.Len() == 0 {
		t.Fatal("logging helpers emitted no output")
	}
}

func TestLoggingExcludesSensitivePayloadsAndRawErrors(t *testing.T) {
	t.Parallel()
	const (
		secret = "sk-secret-api-key"
		prompt = "private user prompt"
		body   = `{"error":"private upstream response body"}`
		keyURL = "https://provider.test/v1?api_key=url-secret"
	)
	wormholeErr := types.NewWormholeError(types.ErrorCodeProvider, "provider rejected request "+secret, false).
		WithProvider("openai").WithModel("gpt-test").WithStatusCode(400).
		WithDetails(strings.Join([]string{secret, body, prompt, keyURL, strings.Repeat("attacker-detail-", 1000)}, " ")).
		WithCause(errors.New(secret))
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	request := types.TextRequest{BaseRequest: types.BaseRequest{Model: keyURL, ProviderOptions: map[string]any{"api_key": secret, "url": keyURL}}, Messages: []types.Message{types.NewUserMessage(prompt)}}
	ctx := context.WithValue(context.Background(), CtxKeyProvider, "openai")
	ctx = context.WithValue(ctx, CtxKeyModel, "gpt-test")
	ctx = context.WithValue(ctx, CtxKeyMethod, "text")
	_, _ = NewDebugTypedLoggingMiddleware(logger).ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Model: "gpt-test", Text: body}, wormholeErr
	})(ctx, request)

	output := buf.String()
	for _, forbidden := range []string{secret, prompt, body, keyURL, "details=", "Cause"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("log output contains sensitive value %q: %s", forbidden, output)
		}
	}
	for _, expected := range []string{"code=PROVIDER_ERROR", `message="provider request failed"`, "provider=openai", "model=gpt-test", "status_code=400", "retryable=false", "request_provider=openai", "request_model=gpt-test", "request_method=text"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("log output missing safe field %q: %s", expected, output)
		}
	}
}

func TestLoggingConstructorsDefaultNilLogger(t *testing.T) {
	t.Parallel()
	if DefaultLoggingConfig(nil).Logger == nil {
		t.Fatal("DefaultLoggingConfig(nil) retained a nil logger")
	}
	if _, err := NewTypedLoggingMiddleware(LoggingConfig{LogTiming: true}).ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) { return nil, nil })(context.Background(), testTextRequest("request")); err != nil {
		t.Fatalf("NewTypedLoggingMiddleware with nil logger returned error: %v", err)
	}
	_ = NewProviderLoggingMiddleware("test", nil)
}

func TestTypedLoggingMiddleware(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mw := NewDebugTypedLoggingMiddleware(newTestLogger(&buf))
	ctx := context.Background()
	if _, err := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Model: "gpt", Text: "ok", Usage: &types.Usage{TotalTokens: 1}}, nil
	})(ctx, testTextRequest("request")); err != nil {
		t.Fatalf("typed text logging error: %v", err)
	}
	stream := make(chan types.StreamChunk, 1)
	stream <- types.StreamChunk{Text: "chunk"}
	close(stream)
	wrapped, err := mw.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) { return stream, nil })(ctx, testTextRequest("request"))
	if err != nil {
		t.Fatalf("typed stream logging error: %v", err)
	}
	for range wrapped {
	}
	wantErr := errors.New("structured")
	if _, err := mw.ApplyStructured(func(context.Context, types.StructuredRequest) (*types.StructuredResponse, error) { return nil, wantErr })(ctx, types.StructuredRequest{BaseRequest: types.BaseRequest{Model: "gpt"}}); !errors.Is(err, wantErr) {
		t.Fatalf("typed structured error = %v, want %v", err, wantErr)
	}
	_, _ = mw.ApplyEmbeddings(func(context.Context, types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return &types.EmbeddingsResponse{Model: "embed"}, nil
	})(ctx, types.EmbeddingsRequest{Model: "embed", Input: []string{"a"}})
	_, _ = mw.ApplyAudio(func(context.Context, types.AudioRequest) (*types.AudioResponse, error) {
		return &types.AudioResponse{Model: "audio"}, nil
	})(ctx, types.AudioRequest{Model: "audio"})
	_, _ = mw.ApplyImage(func(context.Context, types.ImageRequest) (*types.ImageResponse, error) {
		return &types.ImageResponse{Model: "image"}, nil
	})(ctx, types.ImageRequest{Model: "image", Prompt: "draw"})
	if !isNilResponse[*types.TextResponse](nil) || isNilResponse(&types.TextResponse{}) || buf.Len() == 0 {
		t.Fatal("typed logging did not preserve expected response/log behavior")
	}
}

func TestTypedLoggingStreamForwardHonorsContextCancellation(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mw := NewDebugTypedLoggingMiddleware(newTestLogger(&buf))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	upstream := make(chan types.StreamChunk)
	wrapped, err := mw.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) { return upstream, nil })(ctx, testTextRequest("request"))
	if err != nil {
		t.Fatal(err)
	}
	upstream <- types.StreamChunk{Text: "one"}
	<-wrapped
	upstream <- types.StreamChunk{Text: "two"}
	cancel()
	done := make(chan struct{})
	go func() {
		upstream <- types.StreamChunk{Text: "three"}
		upstream <- types.StreamChunk{Text: "four"}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("typed logging stream wrapper stayed blocked after context cancellation")
	}
}

func TestProviderLoggingMiddleware(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mw := NewProviderLoggingMiddleware("test", newTestLogger(&buf))
	ctx := context.Background()
	if _, err := mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		return &types.TextResponse{Usage: &types.Usage{TotalTokens: 3}}, nil
	})(ctx, testTextRequest("hi")); err != nil {
		t.Fatal(err)
	}
	stream := make(chan types.StreamChunk)
	close(stream)
	_, _ = mw.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) { return stream, nil })(ctx, testTextRequest("stream"))
	_, _ = mw.ApplyStructured(func(context.Context, types.StructuredRequest) (*types.StructuredResponse, error) {
		return &types.StructuredResponse{Raw: "{}"}, nil
	})(ctx, types.StructuredRequest{BaseRequest: types.BaseRequest{Model: "structured"}, SchemaName: "schema"})
	_, _ = mw.ApplyEmbeddings(func(context.Context, types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
		return &types.EmbeddingsResponse{Embeddings: []types.Embedding{{Embedding: []float64{1}}}}, nil
	})(ctx, types.EmbeddingsRequest{Model: "embed", Input: []string{"a"}})
	_, _ = mw.ApplyAudio(func(context.Context, types.AudioRequest) (*types.AudioResponse, error) {
		return &types.AudioResponse{}, nil
	})(ctx, types.AudioRequest{Model: "audio"})
	_, _ = mw.ApplyImage(func(context.Context, types.ImageRequest) (*types.ImageResponse, error) {
		return &types.ImageResponse{Images: []types.GeneratedImage{{URL: "https://example.test"}}}, nil
	})(ctx, types.ImageRequest{Model: "image", Prompt: "draw"})
	wantErr := errors.New("stream")
	if _, err := mw.ApplyStream(func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error) { return nil, wantErr })(ctx, testTextRequest("stream")); !errors.Is(err, wantErr) {
		t.Fatalf("provider stream error = %v, want %v", err, wantErr)
	}
	if buf.Len() == 0 {
		t.Fatal("provider logging emitted no output")
	}
}
