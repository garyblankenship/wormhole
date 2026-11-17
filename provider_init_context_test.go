package wormhole

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

type acquisitionWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (ctx *acquisitionWaitContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

func TestProviderInitializationSharesResultAfterWaiterCancellation(t *testing.T) {
	started, resume := make(chan struct{}), make(chan struct{})
	var factories atomic.Int32
	provider := newLifecycleProvider("shared")
	client := New(WithDiscovery(false), WithDefaultProvider("shared"), WithProviderConfig("shared", types.ProviderConfig{}), WithCustomProvider("shared", func(types.ProviderConfig) (types.Provider, error) {
		factories.Add(1)
		close(started)
		<-resume
		return provider, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		handle, err := client.ProviderWithHandleContext(ctx, "shared")
		if handle != nil {
			_ = handle.Close()
		}
		first <- err
	}()
	<-started
	waiting := &acquisitionWaitContext{Context: context.Background(), waiting: make(chan struct{})}
	second := make(chan *ProviderHandle, 1)
	secondError := make(chan error, 1)
	go func() {
		handle, err := client.ProviderWithHandleContext(waiting, "shared")
		second <- handle
		secondError <- err
	}()
	select {
	case <-waiting.waiting:
	case <-time.After(time.Second):
		t.Fatal("second caller did not wait on shared init")
	}
	cancel()
	select {
	case err := <-first:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled caller waited for factory")
	}
	close(resume)
	var handle *ProviderHandle
	select {
	case handle = <-second:
	case <-time.After(time.Second):
		t.Fatal("second caller did not receive provider")
	}
	if err := <-secondError; err != nil {
		t.Fatal(err)
	}
	if handle.Provider != provider || factories.Load() != 1 {
		t.Fatalf("provider=%v factories=%d", handle.Provider, factories.Load())
	}
	client.providersMutex.RLock()
	refs := atomic.LoadInt32(&client.providers["shared"].refCount)
	client.providersMutex.RUnlock()
	if refs != 1 {
		t.Fatalf("refs = %d, want only second caller lease", refs)
	}
	_ = handle.Close()
	_ = handle.Close()
	shutdownCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := client.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	if provider.closeCount.Load() != 1 {
		t.Fatalf("close count = %d", provider.closeCount.Load())
	}
}

func TestCanceledInitializationRemainsOwnedUntilShutdown(t *testing.T) {
	started, resume := make(chan struct{}), make(chan struct{})
	provider := newLifecycleProvider("shared")
	client := New(WithDiscovery(false), WithDefaultProvider("shared"), WithProviderConfig("shared", types.ProviderConfig{}), WithCustomProvider("shared", func(types.ProviderConfig) (types.Provider, error) { close(started); <-resume; return provider, nil }))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := client.ProviderWithHandleContext(ctx, "shared"); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter blocked")
	}
	shutdownCtx, stop := context.WithCancel(context.Background())
	stop()
	if err := client.Shutdown(shutdownCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown = %v", err)
	}
	if provider.closeCount.Load() != 0 {
		t.Fatal("provider closed during construction")
	}
	close(resume)
	finalCtx, finalStop := context.WithTimeout(context.Background(), time.Second)
	defer finalStop()
	if err := client.Shutdown(finalCtx); err != nil {
		t.Fatal(err)
	}
	if provider.closeCount.Load() != 1 {
		t.Fatalf("close count = %d", provider.closeCount.Load())
	}
}

func TestCanceledTemporaryProviderClosesAfterConstruction(t *testing.T) {
	started, resume := make(chan struct{}), make(chan struct{})
	provider := newLifecycleProvider("temporary")
	client := New(WithDiscovery(false), WithDefaultProvider("temporary"), WithProviderConfig("temporary", types.ProviderConfig{}), WithCustomProvider("temporary", func(config types.ProviderConfig) (types.Provider, error) {
		if config.BaseURL != "https://example.test/v1" {
			t.Errorf("BaseURL = %q", config.BaseURL)
		}
		close(started)
		<-resume
		return provider, nil
	}))
	builder := newCommonBuilder(client)
	builder.setBaseURL("https://example.test/v1")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, _, err := builder.getProviderWithBaseURLContext(ctx); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("temporary acquisition blocked")
	}
	close(resume)
	shutdownCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := client.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	if provider.closeCount.Load() != 1 {
		t.Fatalf("close count = %d", provider.closeCount.Load())
	}
}
