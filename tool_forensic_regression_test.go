package wormhole

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestToolExecutorRejectsMalformedArgumentsBeforeHandler(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry()
	var calls atomic.Int32
	registry.Register("lookup", types.NewToolDefinition(types.Tool{
		Name:        "lookup",
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, map[string]any) (any, error) {
		calls.Add(1)
		return "unexpected", nil
	}))

	result := NewToolExecutor(registry).Execute(context.Background(), types.ToolCall{
		ID:             "call-malformed",
		Name:           "lookup",
		ArgsInvalid:    true,
		ArgsParseError: "unexpected end of JSON input",
	})

	if result.ToolCallID != "call-malformed" {
		t.Fatalf("ToolCallID = %q, want correlated call ID", result.ToolCallID)
	}
	if !strings.Contains(result.Error, "malformed arguments") || !strings.Contains(result.Error, "unexpected end of JSON input") {
		t.Fatalf("Error = %q, want malformed argument detail", result.Error)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("handler calls = %d, want 0", got)
	}
}

func TestToolExecutorPermitTracksActualHandlerLifetime(t *testing.T) {
	t.Parallel()

	for _, adaptive := range []bool{false, true} {
		name := "fixed"
		if adaptive {
			name = "adaptive"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			started := make(chan struct{})
			unblock := make(chan struct{})
			var startOnce sync.Once
			var calls atomic.Int32
			registry := NewToolRegistry()
			registry.Register("blocking", types.NewToolDefinition(types.Tool{Name: "blocking"}, func(context.Context, map[string]any) (any, error) {
				calls.Add(1)
				startOnce.Do(func() { close(started) })
				<-unblock
				return "done", nil
			}))

			config := DefaultToolSafetyConfig()
			config.MaxConcurrentTools = 1
			config.ToolTimeout = 20 * time.Millisecond
			config.EnableAdaptiveConcurrency = adaptive
			config.AdaptiveMinCapacity = 1
			config.AdaptiveMaxCapacity = 1
			config.AdaptiveAdjustmentInterval = time.Hour
			executor := NewToolExecutorWithConfig(registry, config)
			if executor.adaptiveLimiter != nil {
				t.Cleanup(executor.adaptiveLimiter.Stop)
			}

			firstResult := make(chan types.ToolResult, 1)
			go func() {
				firstResult <- executor.Execute(context.Background(), types.ToolCall{ID: "first", Name: "blocking"})
			}()
			<-started
			if result := <-firstResult; !strings.Contains(result.Error, "deadline exceeded") {
				t.Fatalf("first Error = %q, want tool timeout", result.Error)
			}
			if got := toolExecutorPermitsInUse(executor); got != 1 {
				t.Fatalf("permits in use after caller cancellation = %d, want 1", got)
			}

			secondCtx, cancelSecond := context.WithCancel(context.Background())
			cancelSecond()
			second := executor.Execute(secondCtx, types.ToolCall{ID: "second", Name: "blocking"})
			if !strings.Contains(second.Error, "concurrency limit") {
				t.Fatalf("second Error = %q, want retained-capacity error", second.Error)
			}
			if got := calls.Load(); got != 1 {
				t.Fatalf("handler calls while first is abandoned = %d, want 1", got)
			}

			close(unblock)
			waitForToolExecutorPermits(t, executor, 0)
			third := executor.Execute(context.Background(), types.ToolCall{ID: "third", Name: "blocking"})
			if third.Error != "" || third.Result != "done" {
				t.Fatalf("third result = %#v, want successful execution", third)
			}
			if adaptive {
				executor.adaptiveLimiter.mu.RLock()
				samples := executor.adaptiveLimiter.sampleCount
				executor.adaptiveLimiter.mu.RUnlock()
				if samples == 0 {
					t.Fatal("adaptive limiter did not record real handler completion latency")
				}
			}
		})
	}
}

func TestToolExecutorPreflightAndPanicDoNotLeakPermits(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry()
	var calls atomic.Int32
	registry.Register("panic", types.NewToolDefinition(types.Tool{
		Name: "panic",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []any{"value"},
			"properties": map[string]any{
				"value": map[string]any{"type": "boolean"},
			},
		},
	}, func(context.Context, map[string]any) (any, error) {
		calls.Add(1)
		panic("boom")
	}))
	config := DefaultToolSafetyConfig()
	config.MaxConcurrentTools = 1
	config.ToolTimeout = 0
	executor := NewToolExecutorWithConfig(registry, config)

	invalid := executor.Execute(context.Background(), types.ToolCall{ID: "invalid", Name: "panic", Arguments: map[string]any{}})
	if !strings.Contains(invalid.Error, "schema validation failed") {
		t.Fatalf("invalid Error = %q", invalid.Error)
	}
	if got := toolExecutorPermitsInUse(executor); got != 0 {
		t.Fatalf("permits after preflight rejection = %d, want 0", got)
	}

	panicked := executor.Execute(context.Background(), types.ToolCall{ID: "panic", Name: "panic", Arguments: map[string]any{"value": true}})
	if !strings.Contains(panicked.Error, "tool handler panicked: boom") {
		t.Fatalf("panic Error = %q", panicked.Error)
	}
	waitForToolExecutorPermits(t, executor, 0)
	if got := calls.Load(); got != 1 {
		t.Fatalf("handler calls = %d, want 1", got)
	}
}

func TestToolRegistryOwnsImmutableSnapshots(t *testing.T) {
	t.Parallel()

	handler := func(context.Context, map[string]any) (any, error) { return "original", nil }
	definition := types.NewToolDefinition(types.Tool{
		Name: "wrong-name",
		InputSchema: map[string]any{
			"properties": map[string]any{"query": map[string]any{"type": "string"}},
		},
		Function: &types.ToolFunction{
			Name:       "wrong-name",
			Parameters: map[string]any{"type": "object"},
		},
	}, handler)
	registry := NewToolRegistry()
	registry.Register("lookup", definition)

	definition.Tool.InputSchema["properties"].(map[string]any)["query"].(map[string]any)["type"] = "number"
	definition.Tool.Function.Parameters["type"] = "array"
	definition.Tool.Name = "mutated"

	first := registry.Get("lookup")
	if first.Tool.Name != "lookup" || first.Tool.Type != "function" {
		t.Fatalf("stored metadata = %#v, want normalized snapshot", first.Tool)
	}
	if got := first.Tool.InputSchema["properties"].(map[string]any)["query"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("stored nested schema type = %v, want string", got)
	}
	if got := first.Tool.Function.Parameters["type"]; got != "object" {
		t.Fatalf("stored function schema type = %v, want object", got)
	}
	result, err := first.Handler(context.Background(), nil)
	if err != nil || result != "original" {
		t.Fatalf("cloned handler result = %v, %v", result, err)
	}

	first.Tool.InputSchema["properties"].(map[string]any)["query"] = "mutated"
	listed := registry.List()
	listed[0].InputSchema["properties"] = "mutated"
	second := registry.Get("lookup")
	if _, ok := second.Tool.InputSchema["properties"].(map[string]any); !ok {
		t.Fatalf("public snapshot mutation changed registry: %#v", second.Tool.InputSchema)
	}
}

func toolExecutorPermitsInUse(executor *ToolExecutor) int {
	if executor.limiter != nil {
		return executor.limiter.InUse()
	}
	if executor.adaptiveLimiter != nil {
		executor.adaptiveLimiter.mu.RLock()
		defer executor.adaptiveLimiter.mu.RUnlock()
		return executor.adaptiveLimiter.limiter.InUse()
	}
	return 0
}

func waitForToolExecutorPermits(t *testing.T, executor *ToolExecutor, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for toolExecutorPermitsInUse(executor) != want {
		if time.Now().After(deadline) {
			t.Fatalf("permits in use = %d, want %d", toolExecutorPermitsInUse(executor), want)
		}
		runtime.Gosched()
	}
}

type toolLoopCountingMiddleware struct {
	applyTextCalls atomic.Int32
	textCalls      atomic.Int32
}

func (m *toolLoopCountingMiddleware) ApplyText(next types.TextHandler) types.TextHandler {
	m.applyTextCalls.Add(1)
	return func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
		m.textCalls.Add(1)
		return next(ctx, request)
	}
}

func (m *toolLoopCountingMiddleware) ApplyStream(next types.StreamHandler) types.StreamHandler {
	return next
}

func (m *toolLoopCountingMiddleware) ApplyStructured(next types.StructuredHandler) types.StructuredHandler {
	return next
}

func (m *toolLoopCountingMiddleware) ApplyEmbeddings(next types.EmbeddingsHandler) types.EmbeddingsHandler {
	return next
}

func (m *toolLoopCountingMiddleware) ApplyAudio(next types.AudioHandler) types.AudioHandler {
	return next
}

func (m *toolLoopCountingMiddleware) ApplyImage(next types.ImageHandler) types.ImageHandler {
	return next
}

func (m *toolLoopCountingMiddleware) ApplyRerank(next types.RerankHandler) types.RerankHandler {
	return next
}

func TestTextBuilderToolLoopUsesMiddlewareForEveryTurn(t *testing.T) {
	t.Parallel()

	provider := &mockToolProvider{responses: []*types.TextResponse{
		{ToolCalls: []types.ToolCall{{ID: "call-1", Name: "lookup", Arguments: map[string]any{}}}},
		{Text: "done"},
	}}
	middleware := &toolLoopCountingMiddleware{}
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithProviderMiddleware(middleware),
		WithDiscovery(false),
	)
	client.RegisterTool("lookup", "lookup", map[string]any{"type": "object"}, func(context.Context, map[string]any) (any, error) {
		return "found", nil
	})

	response, err := client.Text().Model("test-model").Prompt("find it").Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "done" {
		t.Fatalf("response.Text = %q, want done", response.Text)
	}
	if got := middleware.applyTextCalls.Load(); got != 1 {
		t.Fatalf("ApplyText calls = %d, want one wrapper per attempt", got)
	}
	if got := middleware.textCalls.Load(); got != 2 {
		t.Fatalf("middleware text calls = %d, want every tool-loop turn", got)
	}
}
