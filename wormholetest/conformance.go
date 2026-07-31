package wormholetest

import (
	"context"
	"errors"
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// ProviderConformanceConfig controls the standard provider contract checks.
type ProviderConformanceConfig struct {
	Provider        types.Provider
	TextModel       string
	StructuredModel string
	EmbeddingsModel string
	StreamModel     string
	Timeout         time.Duration
	// CheckStreamCancellation opts into a pre-canceled stream context check.
	CheckStreamCancellation bool
}

// RunProviderConformance runs reusable contract checks for custom providers.
//
//nolint:gocyclo // One table-like public conformance harness keeps provider contract failures in one place.
func RunProviderConformance(t *stdtesting.T, cfg ProviderConformanceConfig) {
	t.Helper()
	if cfg.Provider == nil {
		t.Fatal("Provider is nil")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 2 * time.Second
	}
	if cfg.TextModel == "" {
		cfg.TextModel = "test-model"
	}
	if cfg.StructuredModel == "" {
		cfg.StructuredModel = cfg.TextModel
	}
	if cfg.EmbeddingsModel == "" {
		cfg.EmbeddingsModel = cfg.TextModel
	}
	if cfg.StreamModel == "" {
		cfg.StreamModel = cfg.TextModel
	}

	t.Run("identity", func(t *stdtesting.T) {
		if cfg.Provider.Name() == "" {
			t.Fatal("Name returned empty provider name")
		}
		if cfg.Provider.SupportedCapabilities() == nil {
			t.Fatal("SupportedCapabilities returned nil; return an empty slice when no capabilities are supported")
		}
	})

	caps := capabilitySet(cfg.Provider.SupportedCapabilities())
	if caps[types.CapabilityText] || caps[types.CapabilityChat] {
		t.Run("text", func(t *stdtesting.T) {
			ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
			defer cancel()
			resp, err := cfg.Provider.Text(ctx, types.TextRequest{
				BaseRequest: types.BaseRequest{Model: cfg.TextModel},
				Messages:    []types.Message{types.NewUserMessage("hello")},
			})
			if err != nil {
				t.Fatalf("Text returned error for advertised capability: %v", err)
			}
			if resp == nil || resp.Content() == "" {
				t.Fatal("Text returned empty response")
			}
		})
	}
	if caps[types.CapabilityStream] {
		t.Run("stream", func(t *stdtesting.T) {
			ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
			defer cancel()
			stream, err := cfg.Provider.Stream(ctx, types.TextRequest{
				BaseRequest: types.BaseRequest{Model: cfg.StreamModel},
				Messages:    []types.Message{types.NewUserMessage("hello")},
			})
			if err != nil {
				t.Fatalf("Stream returned error for advertised capability: %v", err)
			}
			text, err := CollectStreamText(ctx, stream)
			if err != nil {
				t.Fatalf("Stream produced error: %v", err)
			}
			if text == "" {
				t.Fatal("Stream returned no text")
			}
		})
		if cfg.CheckStreamCancellation {
			t.Run("stream_cancellation", func(t *stdtesting.T) {
				if err := checkStreamCancellation(cfg.Provider, cfg.StreamModel, cfg.Timeout); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	if caps[types.CapabilityStructured] {
		t.Run("structured", func(t *stdtesting.T) {
			ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
			defer cancel()
			resp, err := cfg.Provider.Structured(ctx, types.StructuredRequest{
				BaseRequest: types.BaseRequest{Model: cfg.StructuredModel},
				Messages:    []types.Message{types.NewUserMessage("return json")},
			})
			if err != nil {
				t.Fatalf("Structured returned error for advertised capability: %v", err)
			}
			if resp == nil || resp.Content() == nil {
				t.Fatal("Structured returned empty response")
			}
		})
	}
	if caps[types.CapabilityEmbeddings] {
		t.Run("embeddings", func(t *stdtesting.T) {
			ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
			defer cancel()
			resp, err := cfg.Provider.Embeddings(ctx, types.EmbeddingsRequest{
				Model: cfg.EmbeddingsModel,
				Input: []string{"hello"},
			})
			if err != nil {
				t.Fatalf("Embeddings returned error for advertised capability: %v", err)
			}
			if resp == nil || len(resp.Embeddings) == 0 || len(resp.Embeddings[0].Embedding) == 0 {
				t.Fatal("Embeddings returned no vector data")
			}
		})
	}

	t.Run("close", func(t *stdtesting.T) {
		if err := cfg.Provider.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})
}

func checkStreamCancellation(provider types.Provider, model string, timeout time.Duration) error {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream, err := provider.Stream(ctx, types.TextRequest{
		BaseRequest: types.BaseRequest{Model: model},
		Messages:    []types.Message{types.NewUserMessage("hello")},
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return fmt.Errorf("Stream returned a non-cancellation error for pre-canceled context: %w", err)
	}
	if stream == nil {
		return fmt.Errorf("Stream returned nil channel without a cancellation error")
	}
	wait := timeout
	if wait > 100*time.Millisecond {
		wait = 100 * time.Millisecond
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case chunk, ok := <-stream:
		if !ok {
			return nil
		}
		if errors.Is(chunk.Error, context.Canceled) {
			return nil
		}
		if chunk.Error != nil {
			return fmt.Errorf("Stream produced a non-cancellation error after pre-canceled context: %w", chunk.Error)
		}
		return fmt.Errorf("Stream produced data after pre-canceled context")
	case <-timer.C:
		return fmt.Errorf("Stream neither reported cancellation nor closed promptly")
	}
}

func capabilitySet(capabilities []types.ModelCapability) map[types.ModelCapability]bool {
	set := make(map[types.ModelCapability]bool, len(capabilities))
	for _, capability := range capabilities {
		set[capability] = true
	}
	return set
}
