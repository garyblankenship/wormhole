package wormhole

import (
	"context"
	"errors"
	"testing"

	whtest "github.com/garyblankenship/wormhole/pkg/testing"
	"github.com/garyblankenship/wormhole/pkg/types"
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
