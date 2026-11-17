package wormhole

import (
	"context"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestStreamSingleRouteRetainsPrevalidatedDecision(t *testing.T) {
	useModelRegistry(t, &types.ModelInfo{ID: "valid", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStream}})
	provider := newValidationRecordingProvider("mock")
	var phases []AttemptPhase
	client := New(
		WithDefaultProvider("mock"), WithDiscovery(false),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithAttemptTrace(func(_ context.Context, event AttemptEvent) {
			phases = append(phases, event.Phase)
			if event.Phase == AttemptStarted {
				// Refreshing the registry after admission must not revalidate this attempt.
				types.DefaultModelRegistry.Register(&types.ModelInfo{ID: "valid"})
			}
		}),
	)
	stream, err := client.Text().Model("valid").Prompt("hi").Stream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || chunks[0].Content() != "ok" {
		t.Fatalf("chunks = %#v", chunks)
	}
	if len(phases) != 2 || phases[0] != AttemptStarted || phases[1] != AttemptSuccess {
		t.Fatalf("phases = %v", phases)
	}
}

func TestStreamSingleRouteValidationErrorIsImmediate(t *testing.T) {
	useModelRegistry(t, &types.ModelInfo{ID: "unsupported", Capabilities: []types.ModelCapability{types.CapabilityText}})
	var phases []AttemptPhase
	factoryCalled := false
	client := New(
		WithDefaultProvider("mock"), WithDiscovery(false),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) {
			factoryCalled = true
			return newValidationRecordingProvider("mock"), nil
		}),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithAttemptTrace(func(_ context.Context, event AttemptEvent) { phases = append(phases, event.Phase) }),
	)
	stream, err := client.Text().Model("unsupported").Prompt("hi").Stream(context.Background())
	if err == nil || stream != nil {
		t.Fatalf("Stream = %v, %v; want immediate error", stream, err)
	}
	if factoryCalled {
		t.Fatal("provider acquired for invalid route")
	}
	if len(phases) != 2 || phases[0] != AttemptStarted || phases[1] != AttemptError {
		t.Fatalf("phases = %v", phases)
	}
}

func TestStreamFallbackRetainsValidationBeforeLease(t *testing.T) {
	useModelRegistry(t, &types.ModelInfo{ID: "valid", Capabilities: []types.ModelCapability{types.CapabilityText, types.CapabilityStream}})
	provider := newValidationRecordingProvider("secondary")
	var phases []AttemptPhase
	client := New(
		WithDefaultProvider("mock"), WithDiscovery(false),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return newValidationRecordingProvider("mock"), nil }),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithCustomProvider("secondary", func(types.ProviderConfig) (types.Provider, error) {
			// This route passed capability admission before provider acquisition.
			types.DefaultModelRegistry.Register(&types.ModelInfo{ID: "valid"})
			return provider, nil
		}),
		WithProviderConfig("secondary", types.ProviderConfig{}),
		WithAttemptTrace(func(_ context.Context, event AttemptEvent) { phases = append(phases, event.Phase) }),
	)
	stream, err := client.Text().Model("unknown").Prompt("hi").
		WithProviderFallback(TextRoute{Provider: "secondary", Model: "valid"}).Stream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || chunks[0].Content() != "ok" {
		t.Fatalf("chunks = %#v", chunks)
	}
	if len(phases) != 4 || phases[0] != AttemptStarted || phases[1] != AttemptError || phases[2] != AttemptStarted || phases[3] != AttemptSuccess {
		t.Fatalf("phases = %v", phases)
	}
}
