package wormhole

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
	whtest "github.com/garyblankenship/wormhole/v2/wormholetest"
)

type modelFallbackProvider struct {
	*types.BaseProvider
}

func (p *modelFallbackProvider) Text(_ context.Context, request types.TextRequest) (*types.TextResponse, error) {
	if request.Model == "primary" {
		return nil, errors.New("primary failed")
	}
	return &types.TextResponse{Model: request.Model, Text: "ok", FinishReason: types.FinishReasonStop}, nil
}

func TestAttemptTraceRecordsGenerateFallback(t *testing.T) {
	t.Parallel()
	var events []AttemptEvent
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) {
			return &modelFallbackProvider{BaseProvider: types.NewBaseProvider("mock")}, nil
		}),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithAttemptTrace(func(_ context.Context, event AttemptEvent) {
			events = append(events, event)
		}),
		WithDiscovery(false),
	)

	resp, err := client.Text().Model("primary").WithFallback("fallback").Prompt("hi").Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content() != "ok" {
		t.Fatalf("response = %q", resp.Content())
	}
	if len(events) != 4 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Phase != AttemptStarted || events[1].Phase != AttemptError || events[2].Fallback != true || events[3].Phase != AttemptSuccess {
		t.Fatalf("unexpected events = %#v", events)
	}
}

func TestAttemptTraceRecordsStreamSuccess(t *testing.T) {
	t.Parallel()
	var events []AttemptEvent
	mock := whtest.NewMockProvider("mock").WithStreamChunks(whtest.StreamChunksFrom("ok"))
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", whtest.MockProviderFactory(mock)),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithAttemptTrace(func(_ context.Context, event AttemptEvent) {
			events = append(events, event)
		}),
		WithDiscovery(false),
	)

	stream, err := client.Text().Model("primary").Prompt("hi").Stream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text, err := whtest.CollectStreamText(context.Background(), stream); err != nil || text != "ok" {
		t.Fatalf("stream text=%q err=%v", text, err)
	}
	if len(events) != 2 || !events[0].Stream || events[1].Phase != AttemptSuccess {
		t.Fatalf("events = %#v", events)
	}
}

func TestStreamTraceSuccessPath(t *testing.T) {
	t.Parallel()

	var events []StreamEvent
	mock := whtest.NewMockProvider("mock").WithStreamChunks(whtest.StreamChunksFrom("hello"))
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", whtest.MockProviderFactory(mock)),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithStreamTrace(func(_ context.Context, event StreamEvent) {
			events = append(events, event)
		}),
		WithDiscovery(false),
	)

	stream, err := client.Text().Model("test-model").Prompt("hi").Stream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := whtest.CollectStreamText(context.Background(), stream); err != nil {
		t.Fatal(err)
	}

	// Expect: started + ended
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %#v", len(events), events)
	}
	if events[0].Type != StreamStarted {
		t.Fatalf("first event = %q, want started", events[0].Type)
	}
	if events[1].Type != StreamEnded {
		t.Fatalf("last event = %q, want ended", events[1].Type)
	}
	if events[0].Provider != "mock" || events[0].Model != "test-model" {
		t.Fatalf("started event = %#v", events[0])
	}
}

func TestStreamTraceErrorPath(t *testing.T) {
	t.Parallel()

	var events []StreamEvent
	mock := whtest.NewMockProvider("mock").WithStreamChunks([]types.TextChunk{
		{Text: "partial"},
		{Error: errors.New("stream broke")},
	})
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", whtest.MockProviderFactory(mock)),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithStreamTrace(func(_ context.Context, event StreamEvent) {
			events = append(events, event)
		}),
		WithDiscovery(false),
	)

	stream, err := client.Text().Model("test-model").Prompt("hi").Stream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Drain stream to trigger error
	for range stream {
	}

	// Expect: started + error (terminal from chunk error)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %#v", len(events), events)
	}
	if events[0].Type != StreamStarted {
		t.Fatalf("first event = %q, want started", events[0].Type)
	}
	if events[1].Type != StreamError {
		t.Fatalf("last event = %q, want error", events[1].Type)
	}
	if events[1].Error == nil {
		t.Fatal("expected error in StreamError event")
	}
}

func TestStreamTraceContextCancellation(t *testing.T) {
	parallel := true
	_ = parallel
	t.Parallel()

	var mu sync.Mutex
	var events []StreamEvent
	recordEvent := func(_ context.Context, event StreamEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}

	// Stream that blocks forever — we'll cancel the context
	mock := whtest.NewMockProvider("mock").WithStreamChunks([]types.TextChunk{
		{Text: "first"},
	})
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", whtest.MockProviderFactory(mock)),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithStreamTrace(recordEvent),
		WithDiscovery(false),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := client.Text().Model("test").Prompt("hi").Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Read one chunk then cancel
	for range stream {
		cancel()
		break
	}

	mu.Lock()
	defer mu.Unlock()
	// At least StreamStarted should have fired
	if len(events) == 0 {
		t.Fatal("expected at least StreamStarted event")
	}
	if events[0].Type != StreamStarted {
		t.Fatalf("first event = %q, want started", events[0].Type)
	}
}

func TestStreamTraceNoDuplicateTerminal(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var events []StreamEvent
	recordEvent := func(_ context.Context, event StreamEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}

	mock := whtest.NewMockProvider("mock").WithStreamChunks(whtest.StreamChunksFrom("hello"))
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", whtest.MockProviderFactory(mock)),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithStreamTrace(recordEvent),
		WithDiscovery(false),
	)

	stream, err := client.Text().Model("test").Prompt("hi").Stream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Drain fully
	for range stream {
	}

	mu.Lock()
	defer mu.Unlock()
	// Count terminal events
	var terminal int
	for _, e := range events {
		if e.Type == StreamEnded || e.Type == StreamError {
			terminal++
		}
	}
	if terminal != 1 {
		t.Fatalf("expected exactly 1 terminal event, got %d: %#v", terminal, events)
	}
}
