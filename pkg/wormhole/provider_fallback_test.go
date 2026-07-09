package wormhole

import (
	"context"
	"errors"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

type providerFallbackTextProvider struct {
	*types.BaseProvider
	err      error
	response string
}

type cancelingTextProvider struct {
	*types.BaseProvider
	cancel context.CancelFunc
}

func (p *cancelingTextProvider) Text(ctx context.Context, _ types.TextRequest) (*types.TextResponse, error) {
	p.cancel()
	return nil, ctx.Err()
}

func (p *providerFallbackTextProvider) Text(_ context.Context, request types.TextRequest) (*types.TextResponse, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &types.TextResponse{
		Model:        request.Model,
		Text:         p.response,
		FinishReason: types.FinishReasonStop,
	}, nil
}

func TestGenerateFallsBackToProviderRoute(t *testing.T) {
	primary := &providerFallbackTextProvider{
		BaseProvider: types.NewBaseProvider("primary"),
		err:          errors.New("primary unavailable"),
	}
	secondary := &providerFallbackTextProvider{
		BaseProvider: types.NewBaseProvider("underlying-secondary"),
		response:     "secondary response",
	}

	var events []AttemptEvent
	client := New(
		WithDefaultProvider("primary"),
		WithCustomProvider("primary", func(types.ProviderConfig) (types.Provider, error) {
			return primary, nil
		}),
		WithProviderConfig("primary", types.ProviderConfig{}),
		WithCustomProvider("secondary", func(types.ProviderConfig) (types.Provider, error) {
			return secondary, nil
		}),
		WithProviderConfig("secondary", types.ProviderConfig{}),
		WithAttemptTrace(func(_ context.Context, event AttemptEvent) {
			events = append(events, event)
		}),
		WithDiscovery(false),
	)

	response, err := client.Text().
		Model("primary-model").
		WithFallback("same-provider-model").
		WithProviderFallback(TextRoute{Provider: "secondary", Model: "secondary-model"}).
		Prompt("hello").
		Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := response.Content(); got != "secondary response" {
		t.Fatalf("response = %q, want secondary response", got)
	}

	want := []struct {
		phase    AttemptPhase
		provider string
		model    string
		attempt  int
		fallback bool
	}{
		{AttemptStarted, "primary", "primary-model", 1, false},
		{AttemptError, "primary", "primary-model", 1, false},
		{AttemptStarted, "primary", "same-provider-model", 2, true},
		{AttemptError, "primary", "same-provider-model", 2, true},
		{AttemptStarted, "secondary", "secondary-model", 3, true},
		{AttemptSuccess, "secondary", "secondary-model", 3, true},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %#v", events)
	}
	for i, expected := range want {
		event := events[i]
		if event.Phase != expected.phase || event.Provider != expected.provider || event.Model != expected.model || event.Attempt != expected.attempt || event.Fallback != expected.fallback {
			t.Errorf("event %d = %#v, want phase=%q provider=%q model=%q attempt=%d fallback=%v", i, event, expected.phase, expected.provider, expected.model, expected.attempt, expected.fallback)
		}
	}
}

func TestGenerateDoesNotUseProviderFallbackAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	primary := &cancelingTextProvider{
		BaseProvider: types.NewBaseProvider("primary"),
		cancel:       cancel,
	}
	secondaryFactoryCalls := 0
	client := New(
		WithDefaultProvider("primary"),
		WithCustomProvider("primary", func(types.ProviderConfig) (types.Provider, error) {
			return primary, nil
		}),
		WithProviderConfig("primary", types.ProviderConfig{}),
		WithCustomProvider("secondary", func(types.ProviderConfig) (types.Provider, error) {
			secondaryFactoryCalls++
			return &providerFallbackTextProvider{BaseProvider: types.NewBaseProvider("secondary"), response: "unexpected"}, nil
		}),
		WithProviderConfig("secondary", types.ProviderConfig{}),
		WithDiscovery(false),
	)

	_, err := client.Text().
		Model("primary-model").
		WithProviderFallback(TextRoute{Provider: "secondary", Model: "secondary-model"}).
		Prompt("hello").
		Generate(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate error = %v, want context canceled", err)
	}
	if secondaryFactoryCalls != 0 {
		t.Fatalf("secondary factory calls = %d, want 0", secondaryFactoryCalls)
	}
}

func TestTextBuilderClonePreservesIndependentProviderFallbacks(t *testing.T) {
	builder := New(WithDiscovery(false)).Text().
		WithProviderFallback(TextRoute{Provider: "secondary", Model: "secondary-model"})
	clone := builder.Clone()

	builder.providerFallbacks[0].Model = "changed"
	if got := clone.providerFallbacks[0].Model; got != "secondary-model" {
		t.Fatalf("cloned provider fallback model = %q, want secondary-model", got)
	}
}
