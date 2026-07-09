package wormhole

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

type fallbackStreamProvider struct {
	*types.BaseProvider
	streams map[string]func() (<-chan types.TextChunk, error)
	calls   int
}

type timeoutAwareStreamProvider struct {
	*types.BaseProvider
	closed chan struct{}
}

func (p *timeoutAwareStreamProvider) Stream(ctx context.Context, _ types.TextRequest) (<-chan types.TextChunk, error) {
	stream := make(chan types.TextChunk)
	go func() {
		defer close(stream)
		<-ctx.Done()
		close(p.closed)
	}()
	return stream, nil
}

func newFallbackStreamProvider(streams map[string]func() (<-chan types.TextChunk, error)) *fallbackStreamProvider {
	return newNamedFallbackStreamProvider("fallback-stream", streams)
}

func newNamedFallbackStreamProvider(name string, streams map[string]func() (<-chan types.TextChunk, error)) *fallbackStreamProvider {
	return &fallbackStreamProvider{
		BaseProvider: types.NewBaseProvider(name),
		streams:      streams,
	}
}

func (p *fallbackStreamProvider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	p.calls++
	fn := p.streams[request.Model]
	if fn == nil {
		return nil, errors.New("unexpected model " + request.Model)
	}
	return fn()
}

func streamChunks(chunks ...types.TextChunk) func() (<-chan types.TextChunk, error) {
	return func() (<-chan types.TextChunk, error) {
		ch := make(chan types.TextChunk, len(chunks))
		for _, chunk := range chunks {
			ch <- chunk
		}
		close(ch)
		return ch, nil
	}
}

func collectStreamChunks(t *testing.T, stream <-chan types.StreamChunk) []types.StreamChunk {
	t.Helper()
	var chunks []types.StreamChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func newStreamingFallbackClient(provider *fallbackStreamProvider) *Wormhole {
	return New(
		WithDiscovery(false),
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) {
			return provider, nil
		}),
		WithProviderConfig("mock", types.ProviderConfig{}),
	)
}

func TestTextRequestBuilderStreamFallsBackOnOpenError(t *testing.T) {
	t.Parallel()
	provider := newFallbackStreamProvider(map[string]func() (<-chan types.TextChunk, error){
		"primary": func() (<-chan types.TextChunk, error) {
			return nil, errors.New("open failed")
		},
		"fallback": streamChunks(types.TextChunk{Text: "fallback"}),
	})
	client := newStreamingFallbackClient(provider)

	stream, err := client.Text().Model("primary").Prompt("hi").WithFallback("fallback").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || chunks[0].Content() != "fallback" {
		t.Fatalf("chunks = %#v, want fallback content", chunks)
	}
}

func TestTextRequestBuilderStreamFallsBackOnFirstErrorChunk(t *testing.T) {
	t.Parallel()
	provider := newFallbackStreamProvider(map[string]func() (<-chan types.TextChunk, error){
		"primary":  streamChunks(types.TextChunk{Error: errors.New("rate limited")}),
		"fallback": streamChunks(types.TextChunk{Text: "fallback"}),
	})
	client := newStreamingFallbackClient(provider)

	stream, err := client.Text().Model("primary").Prompt("hi").WithFallback("fallback").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || chunks[0].Content() != "fallback" {
		t.Fatalf("chunks = %#v, want fallback content", chunks)
	}
}

func TestTextRequestBuilderStreamDoesNotFallBackAfterEmission(t *testing.T) {
	t.Parallel()
	provider := newFallbackStreamProvider(map[string]func() (<-chan types.TextChunk, error){
		"primary": streamChunks(
			types.TextChunk{Text: "primary"},
			types.TextChunk{Error: errors.New("late failure")},
		),
		"fallback": streamChunks(types.TextChunk{Text: "fallback"}),
	})
	client := newStreamingFallbackClient(provider)

	stream, err := client.Text().Model("primary").Prompt("hi").WithFallback("fallback").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 2 || chunks[0].Content() != "primary" || !chunks[1].HasError() {
		t.Fatalf("chunks = %#v, want primary content followed by error", chunks)
	}
}

func TestTextRequestBuilderStreamFallsBackToProviderRoute(t *testing.T) {
	primary := newNamedFallbackStreamProvider("primary", map[string]func() (<-chan types.TextChunk, error){
		"primary-model": streamChunks(types.TextChunk{Error: errors.New("primary failed")}),
	})
	secondary := newNamedFallbackStreamProvider("secondary", map[string]func() (<-chan types.TextChunk, error){
		"secondary-model": streamChunks(types.TextChunk{Text: "secondary"}),
	})

	var attempts []AttemptEvent
	var streamEvents []StreamEvent
	client := New(
		WithDiscovery(false),
		WithDefaultProvider("primary"),
		WithCustomProvider("primary", func(types.ProviderConfig) (types.Provider, error) { return primary, nil }),
		WithProviderConfig("primary", types.ProviderConfig{}),
		WithCustomProvider("secondary", func(types.ProviderConfig) (types.Provider, error) { return secondary, nil }),
		WithProviderConfig("secondary", types.ProviderConfig{}),
		WithAttemptTrace(func(_ context.Context, event AttemptEvent) { attempts = append(attempts, event) }),
		WithStreamTrace(func(_ context.Context, event StreamEvent) { streamEvents = append(streamEvents, event) }),
	)

	stream, err := client.Text().
		Model("primary-model").
		Prompt("hi").
		WithProviderFallback(TextRoute{Provider: "secondary", Model: "secondary-model"}).
		Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || chunks[0].Content() != "secondary" {
		t.Fatalf("chunks = %#v, want secondary content", chunks)
	}
	if primary.calls != 1 || secondary.calls != 1 {
		t.Fatalf("calls primary=%d secondary=%d, want 1 each", primary.calls, secondary.calls)
	}
	if len(attempts) != 4 || attempts[0].Provider != "primary" || attempts[0].Model != "primary-model" || attempts[2].Provider != "secondary" || attempts[2].Model != "secondary-model" || attempts[3].Phase != AttemptSuccess {
		t.Fatalf("attempts = %#v", attempts)
	}
	var terminals []StreamEventType
	for _, event := range streamEvents {
		if event.Type == StreamEnded || event.Type == StreamError {
			terminals = append(terminals, event.Type)
		}
	}
	if len(terminals) != 1 || terminals[0] != StreamEnded {
		t.Fatalf("terminal stream events = %#v, want one ended event", terminals)
	}
}

func TestTextRequestBuilderStreamProviderRouteStopsAfterEmission(t *testing.T) {
	primary := newNamedFallbackStreamProvider("primary", map[string]func() (<-chan types.TextChunk, error){
		"primary-model": streamChunks(
			types.TextChunk{Text: "primary"},
			types.TextChunk{Error: errors.New("late failure")},
		),
	})
	secondary := newNamedFallbackStreamProvider("secondary", map[string]func() (<-chan types.TextChunk, error){
		"secondary-model": streamChunks(types.TextChunk{Text: "secondary"}),
	})
	client := New(
		WithDiscovery(false),
		WithDefaultProvider("primary"),
		WithCustomProvider("primary", func(types.ProviderConfig) (types.Provider, error) { return primary, nil }),
		WithProviderConfig("primary", types.ProviderConfig{}),
		WithCustomProvider("secondary", func(types.ProviderConfig) (types.Provider, error) { return secondary, nil }),
		WithProviderConfig("secondary", types.ProviderConfig{}),
	)

	stream, err := client.Text().
		Model("primary-model").
		Prompt("hi").
		WithProviderFallback(TextRoute{Provider: "secondary", Model: "secondary-model"}).
		Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 2 || chunks[0].Content() != "primary" || !chunks[1].HasError() {
		t.Fatalf("chunks = %#v, want primary content followed by error", chunks)
	}
	if secondary.calls != 0 {
		t.Fatalf("secondary calls = %d, want 0", secondary.calls)
	}
}

func TestTextRequestBuilderStreamContinuesAfterProviderLeaseFailure(t *testing.T) {
	primary := newNamedFallbackStreamProvider("primary", map[string]func() (<-chan types.TextChunk, error){
		"primary-model": streamChunks(types.TextChunk{Error: errors.New("primary failed")}),
	})
	secondary := newNamedFallbackStreamProvider("underlying-secondary", map[string]func() (<-chan types.TextChunk, error){
		"secondary-model": streamChunks(types.TextChunk{Text: "secondary"}),
	})

	var attempts []AttemptEvent
	var streamEvents []StreamEvent
	client := New(
		WithDiscovery(false),
		WithDefaultProvider("primary"),
		WithCustomProvider("primary", func(types.ProviderConfig) (types.Provider, error) { return primary, nil }),
		WithProviderConfig("primary", types.ProviderConfig{}),
		WithCustomProvider("secondary", func(types.ProviderConfig) (types.Provider, error) { return secondary, nil }),
		WithProviderConfig("secondary", types.ProviderConfig{}),
		WithAttemptTrace(func(_ context.Context, event AttemptEvent) { attempts = append(attempts, event) }),
		WithStreamTrace(func(_ context.Context, event StreamEvent) { streamEvents = append(streamEvents, event) }),
	)

	stream, err := client.Text().
		Model("primary-model").
		Prompt("hi").
		WithProviderFallback(
			TextRoute{Provider: "uncreatable", Model: "middle-model"},
			TextRoute{Provider: "secondary", Model: "secondary-model"},
		).
		Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || chunks[0].Content() != "secondary" {
		t.Fatalf("chunks = %#v, want secondary content", chunks)
	}

	var middlePhases []AttemptPhase
	for _, event := range attempts {
		if event.Provider == "uncreatable" {
			middlePhases = append(middlePhases, event.Phase)
		}
	}
	if len(middlePhases) != 2 || middlePhases[0] != AttemptStarted || middlePhases[1] != AttemptError {
		t.Fatalf("middle provider attempt phases = %#v, want started then error", middlePhases)
	}
	for _, event := range streamEvents {
		if event.Provider == "uncreatable" && event.Type == StreamError {
			t.Fatalf("lease failure emitted terminal stream error: %#v", event)
		}
	}
	if attempts[len(attempts)-1].Provider != "secondary" || attempts[len(attempts)-1].Phase != AttemptSuccess {
		t.Fatalf("final attempt = %#v, want configured secondary success", attempts[len(attempts)-1])
	}
}

func TestTextRequestBuilderStreamAllAttemptsFailBeforeEmission(t *testing.T) {
	t.Parallel()
	provider := newFallbackStreamProvider(map[string]func() (<-chan types.TextChunk, error){
		"primary": func() (<-chan types.TextChunk, error) {
			return nil, errors.New("open failed")
		},
		"fallback": streamChunks(types.TextChunk{Error: errors.New("fallback failed")}),
	})
	client := newStreamingFallbackClient(provider)

	stream, err := client.Text().Model("primary").Prompt("hi").WithFallback("fallback").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || !chunks[0].HasError() {
		t.Fatalf("chunks = %#v, want one accumulated error chunk", chunks)
	}
}

func TestTextRequestBuilderStreamContextCancellationClosesStream(t *testing.T) {
	t.Parallel()
	blocked := make(chan types.TextChunk)
	provider := newFallbackStreamProvider(map[string]func() (<-chan types.TextChunk, error){
		"primary": func() (<-chan types.TextChunk, error) {
			return blocked, nil
		},
	})
	client := newStreamingFallbackClient(provider)

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.Text().Model("primary").Prompt("hi").Stream(ctx)
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	cancel()
	for range stream {
		t.Fatal("expected stream to close without chunks after cancellation")
	}
}

func TestTextRequestBuilderStreamIdleTimeoutCancelsProviderAttempt(t *testing.T) {
	t.Parallel()

	provider := &timeoutAwareStreamProvider{
		BaseProvider: types.NewBaseProvider("timeout-aware"),
		closed:       make(chan struct{}),
	}
	client := New(
		WithDiscovery(false),
		WithDefaultProvider("timeout-aware"),
		WithCustomProvider("timeout-aware", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
		WithProviderConfig("timeout-aware", types.ProviderConfig{}),
		WithStreamIdleTimeout(25*time.Millisecond),
	)

	stream, err := client.Text().Model("primary").Prompt("hi").Stream(context.Background())
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || chunks[0].Error == nil || !strings.Contains(chunks[0].Error.Error(), "stream idle timeout") {
		t.Fatalf("chunks = %#v, want idle timeout error", chunks)
	}
	select {
	case <-provider.closed:
	case <-time.After(time.Second):
		t.Fatal("idle timeout did not cancel the provider attempt")
	}
}

func TestApplyStreamIdleTimeoutCancellationClosesWhileBlockedOnSend(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	src := make(chan types.StreamChunk)
	out := applyStreamIdleTimeout(ctx, cancel, src, time.Hour)
	sent := make(chan struct{})

	go func() {
		src <- types.StreamChunk{Text: "blocked"}
		close(sent)
	}()

	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("source send did not reach idle-timeout wrapper")
	}

	cancel()

	select {
	case chunk, ok := <-out:
		if ok {
			t.Fatalf("expected wrapper to close without forwarding after cancellation, got %#v", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("idle-timeout wrapper did not close after context cancellation")
	}
}
