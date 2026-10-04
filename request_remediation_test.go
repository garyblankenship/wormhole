package wormhole

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

type remediationCancellationProvider struct {
	*types.BaseProvider
	text func(context.Context, types.TextRequest) (*types.TextResponse, error)
}

func (p *remediationCancellationProvider) Text(ctx context.Context, r types.TextRequest) (*types.TextResponse, error) {
	return p.text(ctx, r)
}

// C1-05: cancellation stops model and provider fallback before additional work.
func TestRemediationC105CanceledGenerationStopsFallback(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"before", "primary", "secondary", "trace"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var factories, calls atomic.Int32
			var events []AttemptEvent
			options := []Option{WithDefaultProvider("primary"), WithDiscovery(false), WithModelValidation(false), WithAttemptTrace(func(_ context.Context, e AttemptEvent) {
				events = append(events, e)
				if scenario == "trace" {
					cancel()
				}
			})}
			for _, name := range []string{"primary", "secondary", "third"} {
				options = append(options, WithProviderConfig(name, types.ProviderConfig{}), WithCustomProvider(name, func(types.ProviderConfig) (types.Provider, error) {
					factories.Add(1)
					return &remediationCancellationProvider{BaseProvider: types.NewBaseProvider(name), text: func(context.Context, types.TextRequest) (*types.TextResponse, error) {
						calls.Add(1)
						if scenario == name {
							cancel()
						}
						return nil, errors.New("failed generation")
					}}, nil
				}))
			}
			client := New(options...)
			defer func() { _ = client.Shutdown(context.Background()) }()
			if scenario == "before" {
				cancel()
			}
			builder := client.Text().Model("first").Prompt("hello").WithProviderFallback(TextRoute{Provider: "secondary", Model: "second"}, TextRoute{Provider: "third", Model: "third"})
			if scenario != "secondary" {
				builder.WithFallback("other")
			}
			_, err := builder.Generate(ctx)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v", err)
			}
			wantFactories, wantCalls, wantEvents := int32(1), int32(1), 1
			switch scenario {
			case "before":
				wantFactories, wantCalls, wantEvents = 0, 0, 0
			case "trace":
				wantCalls = 0
			case "secondary":
				wantFactories, wantCalls, wantEvents = 2, 2, 3
			}
			if factories.Load() != wantFactories || calls.Load() != wantCalls || len(events) != wantEvents {
				t.Fatalf("factories/calls/events = %d/%d/%v, want %d/%d/%d", factories.Load(), calls.Load(), events, wantFactories, wantCalls, wantEvents)
			}
		})
	}
}

// C1-06: implicit provider resolution and explicit routing share the cache scope.
func TestRemediationC106EffectiveProviderIdempotencyScope(t *testing.T) {
	t.Parallel()
	provider := newValidationRecordingProvider("only")
	client := New(WithDiscovery(false), WithModelValidation(false), WithIdempotencyKey("same-effect"), WithProviderConfig("only", types.ProviderConfig{}), WithCustomProvider("only", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }))
	defer func() { _ = client.Shutdown(context.Background()) }()
	for _, name := range []string{"", "only"} {
		_, err := client.Text().Model("model").Prompt("hello").Using(name).Generate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := provider.calledModels(); len(got) != 1 {
		t.Fatalf("provider calls = %v", got)
	}
}

// C1-08: panic followers settle without replay, including a canceled owner.
func TestRemediationC108PanicSettlesFollowersWithoutReplay(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		client := New(WithDiscovery(false), WithIdempotencyKey("panic-effect", time.Hour))
		defer func() { _ = client.Shutdown(context.Background()) }()
		ownerCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		started, gate := make(chan struct{}), make(chan struct{})
		payload := errors.New("private panic detail")
		panicked := make(chan any, 1)
		follower := make(chan error, 1)
		var calls atomic.Int32
		go func() {
			defer func() { panicked <- recover() }()
			_, _ = executeTrackedRequest(ownerCtx, client, "effect", "request", func(context.Context) (int, error) { calls.Add(1); close(started); <-gate; panic(payload) })
		}()
		<-started
		go func() {
			_, err := executeTrackedRequest(context.Background(), client, "effect", "request", func(context.Context) (int, error) { calls.Add(1); return 1, nil })
			follower <- err
		}()
		synctest.Wait()
		cancel()
		close(gate)
		synctest.Wait()
		if got := <-panicked; got != payload {
			t.Fatalf("owner panic = %v", got)
		}
		assertSettled := func(err error) {
			t.Helper()
			var failure *types.WormholeError
			if !errors.As(err, &failure) || failure.Retryable || strings.Contains(err.Error(), payload.Error()) {
				t.Fatalf("settled error = %v", err)
			}
		}
		assertSettled(<-follower)
		<-time.After(time.Minute)
		_, err := executeTrackedRequest(context.Background(), client, "effect", "request", func(context.Context) (int, error) { calls.Add(1); return 1, nil })
		assertSettled(err)
		if calls.Load() != 1 {
			t.Fatalf("effect executions = %d", calls.Load())
		}
	})
}

// C1-09: duplicate model IDs validate the selected provider's metadata.
func TestRemediationC109SelectedProviderModelValidation(t *testing.T) {
	t.Parallel()
	registry := types.NewModelRegistry()
	registry.Register(&types.ModelInfo{ID: "shared", Provider: "alpha", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityFunctions}})
	registry.Register(&types.ModelInfo{ID: "shared", Provider: "beta", Capabilities: []types.ModelCapability{types.CapabilityEmbeddings}, Deprecated: true})
	client := &Wormhole{config: Config{ModelValidation: true, Providers: map[string]types.ProviderConfig{"alpha": {}, "beta": {}}}, modelRegistry: registry}
	if err := client.validateModelAttempt("alpha", "shared", textModelCapabilities, []types.ModelCapability{types.CapabilityFunctions}); err != nil {
		t.Fatal(err)
	}
	if err := client.validateModelAttempt("beta", "shared", []types.ModelCapability{types.CapabilityEmbeddings}, nil); err == nil || !strings.Contains(err.Error(), "deprecated") {
		t.Fatalf("beta validation = %v", err)
	}
	model, _ := registry.Get("shared")
	if model.Provider != "beta" {
		t.Fatalf("global model = %+v", model)
	}
}
