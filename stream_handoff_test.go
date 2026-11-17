package wormhole

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestStreamCancellationDuringProviderAcquisition(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprintf("fallback=%t", fallback), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered, resume := make(chan struct{}), make(chan struct{})
			primary := newNamedFallbackStreamProvider("primary", map[string]func() (<-chan types.TextChunk, error){"first": streamChunks(types.TextChunk{Error: errors.New("try secondary")})})
			acquired := newNamedFallbackStreamProvider("acquired", map[string]func() (<-chan types.TextChunk, error){"first": streamChunks(types.TextChunk{Text: "unexpected"}), "second": streamChunks(types.TextChunk{Text: "unexpected"})})
			factory := func(types.ProviderConfig) (types.Provider, error) { close(entered); <-resume; return acquired, nil }
			primaryFactory := func(types.ProviderConfig) (types.Provider, error) { return primary, nil }
			if !fallback {
				primaryFactory = factory
			}
			client := New(WithDiscovery(false), WithModelValidation(false), WithDefaultProvider("primary"), WithCustomProvider("primary", primaryFactory), WithProviderConfig("primary", types.ProviderConfig{}), WithCustomProvider("secondary", factory), WithProviderConfig("secondary", types.ProviderConfig{}))
			type result struct {
				stream <-chan types.StreamChunk
				err    error
			}
			done := make(chan result, 1)
			go func() {
				builder := client.Text().Model("first").Prompt("hi")
				if fallback {
					builder = builder.WithProviderFallback(TextRoute{Provider: "secondary", Model: "second"})
				}
				stream, err := builder.Stream(ctx)
				done <- result{stream, err}
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("factory not reached")
			}
			cancel()
			close(resume)
			var r result
			select {
			case r = <-done:
			case <-time.After(time.Second):
				t.Fatal("Stream did not return")
			}
			if fallback {
				select {
				case chunk, ok := <-r.stream:
					if ok {
						t.Fatalf("unexpected canceled chunk %#v", chunk)
					}
				case <-time.After(time.Second):
					t.Fatal("stream did not close")
				}
			} else if !errors.Is(r.err, context.Canceled) || r.stream != nil {
				t.Fatalf("Stream = %v, %v", r.stream, r.err)
			}
			if acquired.calls != 0 {
				t.Fatalf("canceled acquired provider called %d times", acquired.calls)
			}
			client.providersMutex.RLock()
			for name, cached := range client.providers {
				if count := atomic.LoadInt32(&cached.refCount); count != 0 {
					t.Errorf("%s lease count = %d", name, count)
				}
			}
			client.providersMutex.RUnlock()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
			defer shutdownCancel()
			if err := client.Shutdown(shutdownCtx); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}
		})
	}
}

func TestStreamCancellationOnPrimaryReleaseSkipsFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	primary := newNamedFallbackStreamProvider("primary", map[string]func() (<-chan types.TextChunk, error){"first": streamChunks(types.TextChunk{Error: errors.New("failed")})})
	acquired := false
	client := New(WithDiscovery(false), WithModelValidation(false), WithDefaultProvider("primary"), WithCustomProvider("primary", func(types.ProviderConfig) (types.Provider, error) { return primary, nil }), WithProviderConfig("primary", types.ProviderConfig{}), WithCustomProvider("secondary", func(types.ProviderConfig) (types.Provider, error) { acquired = true; return primary, nil }), WithProviderConfig("secondary", types.ProviderConfig{}))
	if !client.trackRequest() {
		t.Fatal("request admission failed")
	}
	releases := 0
	out := make(chan types.StreamChunk)
	builder := client.Text()
	go builder.streamWithFallback(ctx, primary, func() { releases++; cancel() }, "primary", &types.TextRequest{}, []string{"first"}, []TextRoute{{Provider: "secondary", Model: "second"}}, false, out)
	select {
	case chunk, ok := <-out:
		if ok {
			t.Fatalf("unexpected chunk %#v", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("stream did not close")
	}
	if releases != 1 || acquired {
		t.Fatalf("releases=%d acquired=%t", releases, acquired)
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := client.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
}

func TestStreamFallbackTraceSequenceAndIdentity(t *testing.T) {
	failure := errors.New("first failed")
	primary := newNamedFallbackStreamProvider("primary-instance", map[string]func() (<-chan types.TextChunk, error){"first": streamChunks(types.TextChunk{Error: failure})})
	secondary := newNamedFallbackStreamProvider("secondary-instance", map[string]func() (<-chan types.TextChunk, error){"second": streamChunks(types.TextChunk{Text: "ok"})})
	var events []string
	var tracedError error
	client := New(WithDiscovery(false), WithModelValidation(false), WithDefaultProvider("primary"), WithCustomProvider("primary", func(types.ProviderConfig) (types.Provider, error) { return primary, nil }), WithProviderConfig("primary", types.ProviderConfig{}), WithCustomProvider("secondary", func(types.ProviderConfig) (types.Provider, error) { return secondary, nil }), WithProviderConfig("secondary", types.ProviderConfig{}), WithAttemptTrace(func(_ context.Context, e AttemptEvent) {
		events = append(events, fmt.Sprintf("attempt/%s/%s/%s/%d", e.Phase, e.Provider, e.Model, e.Attempt))
		if e.Error != nil {
			tracedError = e.Error
		}
	}), WithStreamTrace(func(_ context.Context, e StreamEvent) {
		events = append(events, fmt.Sprintf("stream/%s/%s/%s/%d", e.Type, e.Provider, e.Model, e.Attempt))
	}))
	stream, err := client.Text().Model("first").Prompt("hi").WithProviderFallback(TextRoute{Provider: "secondary", Model: "second"}).Stream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	chunks := collectStreamChunks(t, stream)
	if len(chunks) != 1 || chunks[0].Content() != "ok" {
		t.Fatalf("chunks=%#v", chunks)
	}
	want := []string{
		fmt.Sprintf("attempt/%s/primary-instance/first/1", AttemptStarted),
		fmt.Sprintf("stream/%s/primary-instance/first/1", StreamStarted),
		fmt.Sprintf("attempt/%s/primary-instance/first/1", AttemptError),
		fmt.Sprintf("attempt/%s/secondary/second/2", AttemptStarted),
		fmt.Sprintf("stream/%s/secondary/second/2", StreamStarted),
		fmt.Sprintf("attempt/%s/secondary/second/2", AttemptSuccess),
		fmt.Sprintf("stream/%s/secondary/second/2", StreamEnded),
	}
	if strings.Join(events, "\n") != strings.Join(want, "\n") {
		t.Fatalf("events=%v want=%v", events, want)
	}
	if tracedError != failure {
		t.Fatalf("error=%v want original failure", tracedError)
	}
}
