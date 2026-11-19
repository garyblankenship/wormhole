package wormhole

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/middleware"
	"github.com/garyblankenship/wormhole/v3/types"
	whtest "github.com/garyblankenship/wormhole/v3/wormholetest"
)

type c2FuncProvider struct {
	*types.BaseProvider
	textFn       func(context.Context, types.TextRequest) (*types.TextResponse, error)
	streamFn     func(context.Context, types.TextRequest) (<-chan types.StreamChunk, error)
	embeddingsFn func(context.Context, types.EmbeddingsRequest) (*types.EmbeddingsResponse, error)
	audioFn      func(context.Context, types.AudioRequest) (*types.AudioResponse, error)
}

func (p *c2FuncProvider) Text(ctx context.Context, req types.TextRequest) (*types.TextResponse, error) {
	if p.textFn != nil {
		return p.textFn(ctx, req)
	}
	return p.BaseProvider.Text(ctx, req)
}

func (p *c2FuncProvider) Stream(ctx context.Context, req types.TextRequest) (<-chan types.StreamChunk, error) {
	if p.streamFn != nil {
		return p.streamFn(ctx, req)
	}
	return p.BaseProvider.Stream(ctx, req)
}

func (p *c2FuncProvider) Embeddings(ctx context.Context, req types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	if p.embeddingsFn != nil {
		return p.embeddingsFn(ctx, req)
	}
	return p.BaseProvider.Embeddings(ctx, req)
}

func (p *c2FuncProvider) Audio(ctx context.Context, req types.AudioRequest) (*types.AudioResponse, error) {
	if p.audioFn != nil {
		return p.audioFn(ctx, req)
	}
	return p.BaseProvider.Audio(ctx, req)
}

func newC2Client(t *testing.T, providers map[string]types.Provider, defaultProvider string, extraOpts ...Option) *Wormhole {
	t.Helper()
	opts := []Option{
		WithDefaultProvider(defaultProvider),
		WithDiscovery(false),
	}
	for name, prov := range providers {
		p := prov
		opts = append(opts,
			WithCustomProvider(name, func(types.ProviderConfig) (types.Provider, error) {
				return p, nil
			}),
			WithProviderConfig(name, types.ProviderConfig{}),
		)
	}
	opts = append(opts, extraOpts...)
	client := New(opts...)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestC201_EmbeddingsInputDoesNotAliasCallerSlice(t *testing.T) {
	t.Parallel()

	prov := &c2FuncProvider{
		BaseProvider: types.NewBaseProvider("mock"),
		embeddingsFn: func(_ context.Context, req types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
			out := make([]types.Embedding, len(req.Input))
			for i := range req.Input {
				out[i] = types.Embedding{Index: i, Embedding: []float64{float64(i)}}
			}
			return &types.EmbeddingsResponse{Model: req.Model, Embeddings: out}, nil
		},
	}
	client := newC2Client(t, map[string]types.Provider{"mock": prov}, "mock")

	callerSlice := []string{"alpha", "beta"}
	_, err := client.Embeddings().Model("embed-1").Input(callerSlice...).Generate(context.Background())
	if err != nil {
		t.Fatalf("first Generate failed: %v", err)
	}

	// Reuse pooled request objects with AddInput; must never overwrite callerSlice.
	for range 8 {
		_, err := client.Embeddings().Model("embed-1").AddInput("secret").Generate(context.Background())
		if err != nil {
			t.Fatalf("subsequent Generate failed: %v", err)
		}
	}

	if callerSlice[0] != "alpha" || callerSlice[1] != "beta" {
		t.Fatalf("caller slice was mutated through sync.Pool: %#v", callerSlice)
	}
}

func TestC202_BatchExecuteFirstSnapshotsRequests(t *testing.T) {
	t.Parallel()

	secondDone := make(chan struct{})
	prov := &c2FuncProvider{
		BaseProvider: types.NewBaseProvider("mock"),
		textFn: func(_ context.Context, req types.TextRequest) (*types.TextResponse, error) {
			if req.Model == "second" {
				defer close(secondDone)
			}
			return &types.TextResponse{Model: req.Model, Text: req.Model}, nil
		},
	}
	client := newC2Client(t, map[string]types.Provider{"mock": prov}, "mock")

	// With concurrency 1, request 0 finishes and ExecuteFirst returns immediately
	// while the worker goroutine continues to task index 1.
	batch := client.Batch().
		Concurrency(1).
		Add(client.Text().Model("first").Prompt("p1")).
		Add(client.Text().Model("second").Prompt("p2"))

	resp, err := batch.ExecuteFirst(context.Background())
	if err != nil {
		t.Fatalf("ExecuteFirst failed: %v", err)
	}
	if resp == nil || resp.Model != "first" {
		t.Fatalf("unexpected response: %#v", resp)
	}

	// Mutating the batch immediately after ExecuteFirst must not race or panic.
	batch.Clear()
	batch.Add(client.Text().Model("third").Prompt("p3"))

	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("background batch worker did not complete")
	}
}

func TestC203_StreamIdleTimeoutEmitsTypedErrorAndExitsOnCancel(t *testing.T) {
	t.Parallel()

	t.Run("typed timeout error", func(t *testing.T) {
		t.Parallel()

		prov := &c2FuncProvider{
			BaseProvider: types.NewBaseProvider("mock"),
			streamFn: func(ctx context.Context, _ types.TextRequest) (<-chan types.StreamChunk, error) {
				ch := make(chan types.StreamChunk)
				go func() {
					defer close(ch)
					<-ctx.Done()
				}()
				return ch, nil
			},
		}
		client := newC2Client(t, map[string]types.Provider{"mock": prov}, "mock", WithStreamIdleTimeout(20*time.Millisecond))

		stream, err := client.Text().Model("m1").Prompt("hi").Stream(context.Background())
		if err != nil {
			t.Fatalf("Stream failed: %v", err)
		}

		var gotErr error
		for chunk := range stream {
			if chunk.Error != nil {
				gotErr = chunk.Error
			}
		}
		if gotErr == nil {
			t.Fatal("expected idle timeout error chunk, got nil")
		}
		if !types.IsTimeoutError(gotErr) {
			t.Fatalf("expected types.IsTimeoutError(err) == true, got %T: %v", gotErr, gotErr)
		}
		if !errors.Is(gotErr, types.ErrTimeout) {
			t.Fatalf("expected errors.Is(err, types.ErrTimeout) == true, got %v", gotErr)
		}
		whErr, ok := types.AsWormholeError(gotErr)
		if !ok || !whErr.Retryable || whErr.Code != types.ErrorCodeTimeout {
			t.Fatalf("unexpected WormholeError: %#v (ok=%v)", whErr, ok)
		}
	})

	t.Run("exits promptly on ctx cancel", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		src := make(chan types.StreamChunk)
		out := applyStreamIdleTimeout(ctx, cancel, src, 10*time.Second)

		cancel()

		select {
		case _, ok := <-out:
			if ok {
				t.Fatal("expected closed channel after context cancellation")
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("applyStreamIdleTimeout goroutine did not exit promptly on context cancellation")
		}
	})
}

func TestC204_StreamTraceTerminalOnCancelAndMetadataOnPreChunkFailure(t *testing.T) {
	t.Parallel()

	t.Run("context cancellation emits terminal StreamError", func(t *testing.T) {
		t.Parallel()

		var mu sync.Mutex
		var events []StreamEvent
		terminalDone := make(chan struct{})
		var once sync.Once

		prov := &c2FuncProvider{
			BaseProvider: types.NewBaseProvider("mock"),
			streamFn: func(ctx context.Context, _ types.TextRequest) (<-chan types.StreamChunk, error) {
				ch := make(chan types.StreamChunk, 1)
				go func() {
					defer close(ch)
					ch <- types.StreamChunk{Text: "first"}
					<-ctx.Done()
				}()
				return ch, nil
			},
		}
		client := newC2Client(t, map[string]types.Provider{"mock": prov}, "mock", WithStreamTrace(func(_ context.Context, ev StreamEvent) {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
			if ev.Type == StreamEnded || ev.Type == StreamError {
				once.Do(func() { close(terminalDone) })
			}
		}))

		ctx, cancel := context.WithCancel(context.Background())
		stream, err := client.Text().Model("m1").Prompt("hi").Stream(ctx)
		if err != nil {
			t.Fatalf("Stream failed: %v", err)
		}

		// Read first chunk then cancel.
		<-stream
		cancel()
		for range stream {
		}

		select {
		case <-terminalDone:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for terminal StreamEvent on context cancellation")
		}

		mu.Lock()
		defer mu.Unlock()
		var started, ended, errored int
		var errEvent StreamEvent
		for _, ev := range events {
			switch ev.Type {
			case StreamStarted:
				started++
			case StreamEnded:
				ended++
			case StreamError:
				errored++
				errEvent = ev
			}
		}
		if started != 1 || ended != 0 || errored != 1 {
			t.Fatalf("expected 1 started, 0 ended, 1 error; got %#v", events)
		}
		if !errors.Is(errEvent.Error, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", errEvent.Error)
		}
		if errEvent.Provider != "mock" || errEvent.Model != "m1" || errEvent.Attempt != 1 {
			t.Fatalf("unexpected terminal event metadata: %#v", errEvent)
		}
	})
}

func TestC204_StreamTraceMetadataOnPreChunkFailure(t *testing.T) {
	t.Parallel()

	t.Run("pre-chunk fallback emits StreamStarted per attempt and populates metadata on failure", func(t *testing.T) {
		t.Parallel()

		var mu sync.Mutex
		var events []StreamEvent

		prov := &c2FuncProvider{
			BaseProvider: types.NewBaseProvider("mock"),
			streamFn: func(_ context.Context, req types.TextRequest) (<-chan types.StreamChunk, error) {
				ch := make(chan types.StreamChunk, 1)
				ch <- types.StreamChunk{Error: errors.New("boom:" + req.Model)}
				close(ch)
				return ch, nil
			},
		}
		client := newC2Client(t, map[string]types.Provider{"mock": prov}, "mock", WithStreamTrace(func(_ context.Context, ev StreamEvent) {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		}))

		stream, err := client.Text().Model("m1").WithFallback("m2").Prompt("hi").Stream(context.Background())
		if err != nil {
			t.Fatalf("Stream failed: %v", err)
		}
		for range stream {
		}

		mu.Lock()
		defer mu.Unlock()
		var started, errored int
		var errEvent StreamEvent
		for _, ev := range events {
			switch ev.Type {
			case StreamStarted:
				started++
			case StreamError:
				errored++
				errEvent = ev
			}
		}
		if started != 2 {
			t.Fatalf("expected StreamStarted once per opened attempt, got %d: %#v", started, events)
		}
		if errored != 1 {
			t.Fatalf("expected StreamError exactly once, got %d: %#v", errored, events)
		}
		if errEvent.Provider != "mock" || errEvent.Model != "m2" || errEvent.Attempt != 2 {
			t.Fatalf("expected populated metadata on pre-chunk failure, got %#v", errEvent)
		}
	})
}

func TestC205_StreamAndAccumulateRecordsContextCancellation(t *testing.T) {
	t.Parallel()

	t.Run("records ctx.Err when stream closes on cancel", func(t *testing.T) {
		t.Parallel()

		prov := &c2FuncProvider{
			BaseProvider: types.NewBaseProvider("mock"),
			streamFn: func(ctx context.Context, _ types.TextRequest) (<-chan types.StreamChunk, error) {
				ch := make(chan types.StreamChunk, 1)
				go func() {
					defer close(ch)
					ch <- types.StreamChunk{Text: "partial"}
					<-ctx.Done()
				}()
				return ch, nil
			},
		}
		client := newC2Client(t, map[string]types.Provider{"mock": prov}, "mock")

		ctx, cancel := context.WithCancel(context.Background())
		chunks, getResult, err := client.Text().Model("m1").Prompt("hi").StreamAndAccumulate(ctx)
		if err != nil {
			t.Fatalf("StreamAndAccumulate failed: %v", err)
		}

		first := <-chunks
		if first.Content() != "partial" {
			t.Fatalf("first chunk = %q, want partial", first.Content())
		}
		cancel()
		for range chunks {
		}

		text, streamErr := getResult()
		if text != "partial" {
			t.Fatalf("accumulated text = %q, want partial", text)
		}
		if !errors.Is(streamErr, context.Canceled) {
			t.Fatalf("expected context.Canceled from getResult(), got %v", streamErr)
		}
	})

	t.Run("records ctx.Err when consumer abandons channel", func(t *testing.T) {
		t.Parallel()

		mock := whtest.NewMockProvider("mock").WithStreamChunks([]types.TextChunk{
			{Text: "one"},
			{Text: "two"},
			{Text: "three"},
		})
		client := newC2Client(t, map[string]types.Provider{"mock": mock}, "mock")

		ctx, cancel := context.WithCancel(context.Background())
		chunks, getResult, err := client.Text().Model("m1").Prompt("hi").StreamAndAccumulate(ctx)
		if err != nil {
			t.Fatalf("StreamAndAccumulate failed: %v", err)
		}

		<-chunks
		cancel()
		// Wait for goroutine to close accumulated channel.
		for range chunks {
		}

		_, streamErr := getResult()
		if !errors.Is(streamErr, context.Canceled) {
			t.Fatalf("expected context.Canceled from getResult(), got %v", streamErr)
		}
	})
}

func TestC206_ProviderFallbackDoesNotLeakPrimaryBaseURL(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var primaryGenBaseURL, fallbackGenBaseURL string
	var primaryStreamBaseURL, fallbackStreamBaseURL string

	primaryProv := &c2FuncProvider{
		BaseProvider: types.NewBaseProvider("primary"),
		textFn: func(ctx context.Context, _ types.TextRequest) (*types.TextResponse, error) {
			mu.Lock()
			primaryGenBaseURL, _ = ctx.Value(middleware.CtxKeyBaseURLOverride).(string)
			mu.Unlock()
			return nil, errors.New("primary down")
		},
		streamFn: func(ctx context.Context, _ types.TextRequest) (<-chan types.StreamChunk, error) {
			mu.Lock()
			primaryStreamBaseURL, _ = ctx.Value(middleware.CtxKeyBaseURLOverride).(string)
			mu.Unlock()
			return nil, errors.New("primary stream down")
		},
	}
	fallbackProv := &c2FuncProvider{
		BaseProvider: types.NewBaseProvider("fallback"),
		textFn: func(ctx context.Context, req types.TextRequest) (*types.TextResponse, error) {
			mu.Lock()
			fallbackGenBaseURL, _ = ctx.Value(middleware.CtxKeyBaseURLOverride).(string)
			mu.Unlock()
			return &types.TextResponse{Model: req.Model, Text: "ok"}, nil
		},
		streamFn: func(ctx context.Context, _ types.TextRequest) (<-chan types.StreamChunk, error) {
			mu.Lock()
			fallbackStreamBaseURL, _ = ctx.Value(middleware.CtxKeyBaseURLOverride).(string)
			mu.Unlock()
			ch := make(chan types.StreamChunk, 1)
			ch <- types.StreamChunk{Text: "ok"}
			close(ch)
			return ch, nil
		},
	}

	client := newC2Client(t, map[string]types.Provider{
		"primary":  primaryProv,
		"fallback": fallbackProv,
	}, "primary")

	const primaryOverride = "https://primary.override.test/v1"
	resp, err := client.Text().
		Using("primary").
		BaseURL(primaryOverride).
		Model("m1").
		WithProviderFallback(TextRoute{Provider: "fallback", Model: "m2"}).
		Prompt("hi").
		Generate(context.Background())
	if err != nil {
		t.Fatalf("Generate with fallback failed: %v", err)
	}
	if resp.Content() != "ok" {
		t.Fatalf("response = %q, want ok", resp.Content())
	}

	stream, err := client.Text().
		Using("primary").
		BaseURL(primaryOverride).
		Model("m1").
		WithProviderFallback(TextRoute{Provider: "fallback", Model: "m2"}).
		Prompt("hi").
		Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream with fallback failed: %v", err)
	}
	if text, err := whtest.CollectStreamText(context.Background(), stream); err != nil || text != "ok" {
		t.Fatalf("stream text=%q err=%v", text, err)
	}

	mu.Lock()
	defer mu.Unlock()
	if primaryGenBaseURL != primaryOverride || primaryStreamBaseURL != primaryOverride {
		t.Fatalf("primary baseURL overrides = (%q, %q), want %q", primaryGenBaseURL, primaryStreamBaseURL, primaryOverride)
	}
	if fallbackGenBaseURL != "" || fallbackStreamBaseURL != "" {
		t.Fatalf("fallback leaked primary baseURL override: generate=%q stream=%q", fallbackGenBaseURL, fallbackStreamBaseURL)
	}
}

func TestC207_EmbeddingsBuilderSpentGuards(t *testing.T) {
	t.Parallel()

	prov := &c2FuncProvider{
		BaseProvider: types.NewBaseProvider("mock"),
		embeddingsFn: func(_ context.Context, req types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
			return &types.EmbeddingsResponse{
				Model:      req.Model,
				Embeddings: []types.Embedding{{Index: 0, Embedding: []float64{1.0}}},
			}, nil
		},
	}
	client := newC2Client(t, map[string]types.Provider{"mock": prov}, "mock")

	builder := client.Embeddings().Model("embed-1").Input("hello")
	if _, err := builder.Generate(context.Background()); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// All setters on a spent builder must be safe no-ops without panicking.
	builder.
		Model("embed-2").
		Input("a", "b").
		AddInput("c").
		Dimensions(16).
		EncodingFormat(types.EmbeddingEncodingBase64).
		ProviderOptions(map[string]any{"k": "v"})

	err := builder.Validate()
	if err == nil {
		t.Fatal("expected validation error from Validate() on spent builder, got nil")
	}
	if !strings.Contains(err.Error(), "already used") {
		t.Fatalf("expected already-used validation error, got: %v", err)
	}
}

func TestC208_SpeechToTextPreservesAudioFormat(t *testing.T) {
	t.Parallel()

	var captured types.AudioRequest
	prov := &c2FuncProvider{
		BaseProvider: types.NewBaseProvider("mock"),
		audioFn: func(_ context.Context, req types.AudioRequest) (*types.AudioResponse, error) {
			captured = req
			return &types.AudioResponse{Model: req.Model, Text: "transcribed"}, nil
		},
	}
	client := newC2Client(t, map[string]types.Provider{"mock": prov}, "mock")

	resp, err := client.Audio().
		SpeechToText().
		Model("whisper-1").
		Audio([]byte("fake-mp3-bytes"), "mp3").
		Transcribe(context.Background())
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}
	if resp.Text != "transcribed" {
		t.Fatalf("resp.Text = %q, want transcribed", resp.Text)
	}
	if captured.AudioFormat != "mp3" {
		t.Fatalf("captured AudioRequest.AudioFormat = %q, want mp3", captured.AudioFormat)
	}
}

func TestC209_SimpleFactoryWithCacheLazyAndIsolated(t *testing.T) {
	t.Parallel()

	opt := Quick.WithCache(time.Minute)

	cfg1 := &Config{}
	cfg2 := &Config{}
	opt(cfg1)
	opt(cfg2)

	if len(cfg1.Closers) != 1 || len(cfg2.Closers) != 1 {
		t.Fatalf("expected 1 closer per config, got %d and %d", len(cfg1.Closers), len(cfg2.Closers))
	}
	if cfg1.Closers[0] == cfg2.Closers[0] {
		t.Fatal("expected distinct MemoryCache instances per Option application, got shared instance")
	}
	_ = cfg1.Closers[0].Close()
	_ = cfg2.Closers[0].Close()
}
