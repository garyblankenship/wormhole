package wormhole

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/middleware"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestV330PrewarmBypassesLocalMemoization(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("X-Request-ID", "upstream")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "prewarm", "status": "completed", "usage": map[string]any{"input_tokens": 10, "output_tokens": 0, "total_tokens": 10}})
	}))
	t.Cleanup(upstream.Close)
	cache := middleware.NewMemoryCache(8)
	t.Cleanup(func() { _ = cache.Close() })
	cacheMW := middleware.NewTypedCacheMiddleware(middleware.CacheConfig{Cache: cache, TTL: time.Hour, KeyGenerator: func(any) (string, error) { return "matching-local-entry", nil }})
	cache.Set("text:openai:matching-local-entry", &types.TextResponse{Text: "cached"}, time.Hour)
	client := New(WithOpenAI("test-key", types.ProviderConfig{BaseURL: upstream.URL, UseResponsesAPI: true}), WithDiscovery(false), WithIdempotencyKey("same", time.Hour), WithProviderMiddleware(cacheMW))
	t.Cleanup(func() { _ = client.Close() })
	builder := client.Text().Using("openai").Model("fixture").Prompt("warm").ProviderOptions(map[string]any{"max_output_tokens": 0})
	for range 2 {
		response, err := builder.Generate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if response.Text != "" || response.ResponseMetadata == nil || response.ResponseMetadata.RequestID != "upstream" {
			t.Fatalf("unexpected prewarm: %#v", response)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls.Load())
	}
	if client.GetIdempotencyCacheStats().Entries != 0 {
		t.Fatal("prewarm entered idempotency cache")
	}
}

func TestV330FallbackOwnershipAndSignatureIsolation(t *testing.T) {
	t.Parallel()
	client := New(WithProviderConfig("primary", types.ProviderConfig{}), WithProviderConfig("secondary", types.ProviderConfig{}), WithDiscovery(false))
	t.Cleanup(func() { _ = client.Close() })
	ref := &types.FileReferenceMedia{Provider: "primary", Kind: "document", ID: "opaque"}
	messages := []types.Message{&types.UserMessage{Content: "read", Media: []types.Media{ref}}, &types.AssistantMessage{Thinking: &types.Thinking{Signature: "signed"}, ToolCalls: []types.ToolCall{{ThoughtSignature: "tool-signed"}}}}
	if err := client.validateFileOwnership(messages, "primary"); err != nil {
		t.Fatal(err)
	}
	if err := client.validateFileOwnership(messages, "secondary"); err == nil {
		t.Fatal("cross-provider file accepted")
	}
	own := prepareAttemptMessages(messages, "primary", "primary")[1].(*types.AssistantMessage)
	if own.Thinking.Signature != "signed" || own.ToolCalls[0].ThoughtSignature != "tool-signed" {
		t.Fatal("same-provider signatures lost")
	}
	foreign := prepareAttemptMessages(messages, "primary", "secondary")[1].(*types.AssistantMessage)
	if foreign.Thinking.Signature != "" || foreign.ToolCalls[0].ThoughtSignature != "" {
		t.Fatal("foreign signature leaked")
	}
	if messages[1].(*types.AssistantMessage).Thinking.Signature != "signed" {
		t.Fatal("caller snapshot mutated")
	}
}

func TestV330BuilderReasoningSnapshot(t *testing.T) {
	t.Parallel()
	no := false
	request := &types.TextRequest{BaseRequest: types.BaseRequest{Reasoning: &types.Reasoning{IncludeThoughts: &no}}}
	snapshot := cloneTextRequest(request)
	no = true
	if *snapshot.Reasoning.IncludeThoughts {
		t.Fatal("reasoning pointer shared")
	}
	cfg := types.ProviderConfig{ResponseHeaderNames: []string{"X-Custom"}}
	copy := cloneProviderConfig(cfg)
	cfg.ResponseHeaderNames[0] = "Changed"
	if copy.ResponseHeaderNames[0] != "X-Custom" {
		t.Fatal("config header slice shared")
	}
}
