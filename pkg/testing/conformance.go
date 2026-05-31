package testing

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ProviderConformanceConfig controls the standard provider contract checks.
type ProviderConformanceConfig struct {
	Provider        types.Provider
	TextModel       string
	StructuredModel string
	EmbeddingsModel string
	StreamModel     string
	Timeout         time.Duration
}

// RunProviderConformance runs reusable contract checks for custom providers.
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

func capabilitySet(capabilities []types.ModelCapability) map[types.ModelCapability]bool {
	set := make(map[types.ModelCapability]bool, len(capabilities))
	for _, capability := range capabilities {
		set[capability] = true
	}
	return set
}
