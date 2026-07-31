package wormhole

import (
	"context"
	"runtime"
	"sync"
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

func TestProviderHandleIsInvalidatedByShutdownAndCloseRemainsHarmless(t *testing.T) {
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
	if err := client.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if got := provider.closeCount.Load(); got != 1 {
		t.Fatalf("provider close count = %d, want 1", got)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("first handle.Close after shutdown: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("second handle.Close after shutdown: %v", err)
	}
	if got := provider.closeCount.Load(); got != 1 {
		t.Fatalf("provider close count after handle closes = %d, want 1", got)
	}
}

func TestClientToolAdmissionBudgetIsShared(t *testing.T) {
	t.Parallel()

	config := DefaultToolSafetyConfig()
	config.MaxConcurrentTools = 1
	client := New(WithToolSafetyConfig(config), WithDiscovery(false))
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })

	registry := NewToolRegistry()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	registry.Register("tool", types.NewToolDefinition(types.Tool{Name: "tool"}, func(context.Context, map[string]any) (any, error) {
		entered <- struct{}{}
		<-release
		return "ok", nil
	}))
	first := client.newToolExecutor(registry)
	second := client.newToolExecutor(registry)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		first.Execute(context.Background(), types.ToolCall{ID: "first", Name: "tool"})
	}()
	<-entered
	go func() {
		defer wg.Done()
		second.Execute(context.Background(), types.ToolCall{ID: "second", Name: "tool"})
	}()

	select {
	case <-entered:
		t.Fatal("second executor bypassed the client-owned admission budget")
	case <-time.After(25 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	wg.Wait()
}

type blockingAgentProvider struct {
	*lifecycleProvider
	entered chan struct{}
	release chan struct{}
}

func (p *blockingAgentProvider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityText, types.CapabilityChat, types.CapabilityFunctions}
}

func (p *blockingAgentProvider) Text(context.Context, types.TextRequest) (*types.TextResponse, error) {
	close(p.entered)
	<-p.release
	return &types.TextResponse{Text: "done"}, nil
}

func TestShutdownWaitsForActiveAgentBeforeStoppingToolAdmission(t *testing.T) {
	t.Parallel()

	provider := &blockingAgentProvider{
		lifecycleProvider: newLifecycleProvider("blocking"),
		entered:           make(chan struct{}),
		release:           make(chan struct{}),
	}
	config := DefaultToolSafetyConfig()
	config.EnableAdaptiveConcurrency = true
	config.MaxConcurrentTools = 1
	config.AdaptiveMinCapacity = 1
	config.AdaptiveMaxCapacity = 1
	config.AdaptiveAdjustmentInterval = time.Hour
	client := New(
		WithDefaultProvider("blocking"),
		WithCustomProvider("blocking", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
		WithProviderConfig("blocking", types.ProviderConfig{}),
		WithToolSafetyConfig(config),
		WithModelValidation(false),
		WithDiscovery(false),
	)
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(provider.release) }) })
	agentDone := make(chan error, 1)
	go func() {
		_, err := client.Agent().
			Model("test").
			AddTool("tool", "test", nil, func(context.Context, map[string]any) (any, error) { return nil, nil }).
			Run(context.Background(), "run")
		agentDone <- err
	}()
	<-provider.entered

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- client.Shutdown(context.Background()) }()
	for !client.IsShuttingDown() {
		runtime.Gosched()
	}
	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned while agent was active: %v", err)
	default:
	}
	select {
	case <-client.toolBudget.adaptiveLimiter.stopChan:
		t.Fatal("shared tool admission stopped while agent was active")
	default:
	}

	releaseOnce.Do(func() { close(provider.release) })
	if err := <-agentDone; err != nil {
		t.Fatalf("Agent.Run: %v", err)
	}
	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case <-client.toolBudget.adaptiveLimiter.stopChan:
	default:
		t.Fatal("shared tool admission still running after shutdown")
	}
}
