package wormhole

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

type lifecycleProvider struct {
	*types.BaseProvider
	closeCount atomic.Int32
}

func newLifecycleProvider(name string) *lifecycleProvider {
	return &lifecycleProvider{BaseProvider: types.NewBaseProvider(name)}
}

func (p *lifecycleProvider) Close() error {
	p.closeCount.Add(1)
	return nil
}

func (p *lifecycleProvider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityText, types.CapabilityChat}
}

// TestRawProviderPinnedUntilShutdown distinguishes the public raw reference
// from a lease: both age and count cleanup must leave it open until shutdown.
func TestRawProviderPinnedUntilShutdown(t *testing.T) {
	t.Parallel()

	for _, cleanup := range []struct {
		name string
		run  func(*Wormhole)
	}{
		{"age", func(client *Wormhole) { client.CleanupStaleProviders(0, 0) }},
		{"max-count", func(client *Wormhole) { client.CleanupStaleProviders(time.Hour, 1) }},
	} {
		t.Run(cleanup.name, func(t *testing.T) {
			t.Parallel()
			provider := newLifecycleProvider("raw")
			client := New(
				WithDefaultProvider("raw"),
				WithCustomProvider("raw", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
				WithProviderConfig("raw", types.ProviderConfig{}),
				WithDiscovery(false),
			)
			if _, err := client.Provider("raw"); err != nil {
				t.Fatalf("Provider: %v", err)
			}
			if cleanup.name == "max-count" {
				other := newLifecycleProvider("other")
				client.providerFactories["other"] = func(types.ProviderConfig) (types.Provider, error) { return other, nil }
				client.config.Providers["other"] = types.ProviderConfig{}
				if _, err := client.ProviderWithHandle("other"); err != nil {
					t.Fatalf("ProviderWithHandle(other): %v", err)
				}
			}

			cleanup.run(client)
			if got := provider.closeCount.Load(); got != 0 {
				t.Fatalf("raw provider close count after cleanup = %d, want 0", got)
			}
			if err := client.Shutdown(context.Background()); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}
			if got := provider.closeCount.Load(); got != 1 {
				t.Fatalf("raw provider close count after shutdown = %d, want 1", got)
			}
		})
	}
}

func TestReleasedProviderHandleRemainsEvictable(t *testing.T) {
	t.Parallel()
	provider := newLifecycleProvider("leased")
	client := New(
		WithDefaultProvider("leased"),
		WithCustomProvider("leased", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
		WithProviderConfig("leased", types.ProviderConfig{}),
		WithDiscovery(false),
	)
	handle, err := client.ProviderWithHandle("leased")
	if err != nil {
		t.Fatalf("ProviderWithHandle: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("handle.Close: %v", err)
	}
	client.CleanupStaleProviders(0, 0)
	if got := provider.closeCount.Load(); got != 1 {
		t.Fatalf("released handle provider close count = %d, want 1", got)
	}
}
