package wormhole

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestToolExecutorRejectsInvalidBatchBeforeSideEffects(t *testing.T) {
	t.Parallel()
	registry := NewToolRegistry()
	var calls atomic.Int32
	registry.Register("tool", types.NewToolDefinition(types.Tool{Name: "tool"}, func(context.Context, map[string]any) (any, error) {
		calls.Add(1)
		return "unexpected", nil
	}))
	config := DefaultToolSafetyConfig()
	config.MaxToolCallsPerRound = 2
	executor := NewToolExecutorWithConfig(registry, config)
	t.Cleanup(executor.Stop)
	t.Cleanup(func() {
		if calls.Load() != 0 {
			t.Fatalf("handler calls = %d, want 0", calls.Load())
		}
	})

	tests := []struct {
		name  string
		calls []types.ToolCall
		want  string
	}{
		{
			name: "oversized",
			calls: []types.ToolCall{
				{ID: "one", Name: "tool"},
				{ID: "two", Name: "tool"},
				{ID: "three", Name: "tool"},
			},
			want: "exceeds limit",
		},
		{
			name:  "empty ID",
			calls: []types.ToolCall{{ID: "", Name: "tool"}},
			want:  "ID is empty",
		},
		{
			name: "duplicate normalized ID",
			calls: []types.ToolCall{
				{ID: "same:id", Name: "tool"},
				{ID: "same?id", Name: "tool"},
			},
			want: "duplicate normalized",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			results := executor.ExecuteAll(context.Background(), test.calls)
			if len(results) != 1 || !strings.Contains(results[0].Error, test.want) {
				t.Fatalf("results = %#v, want one %q error", results, test.want)
			}
		})
	}
}

func TestToolExecutorWorkerCountAndOrderingAreBounded(t *testing.T) {
	t.Parallel()
	registry := NewToolRegistry()
	var active atomic.Int32
	var maximum atomic.Int32
	entered := make(chan int, 12)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	registry.Register("tool", types.NewToolDefinition(types.Tool{Name: "tool"}, func(_ context.Context, args map[string]any) (any, error) {
		current := active.Add(1)
		index := args["index"].(int)
		for {
			old := maximum.Load()
			if current <= old || maximum.CompareAndSwap(old, current) {
				break
			}
		}
		entered <- index
		<-release
		active.Add(-1)
		return index, nil
	}))
	config := DefaultToolSafetyConfig()
	config.MaxConcurrentTools = 3
	config.MaxToolCallsPerRound = 20
	executor := NewToolExecutorWithConfig(registry, config)
	t.Cleanup(executor.Stop)

	calls := make([]types.ToolCall, 12)
	for i := range calls {
		calls[i] = types.ToolCall{
			ID:        string(rune('a' + i)),
			Name:      "tool",
			Arguments: map[string]any{"index": i},
		}
	}
	resultsDone := make(chan []types.ToolResult, 1)
	go func() { resultsDone <- executor.ExecuteAll(context.Background(), calls) }()
	for range config.MaxConcurrentTools {
		<-entered
	}
	if got := active.Load(); got != int32(config.MaxConcurrentTools) {
		t.Fatalf("active handlers = %d, want %d", got, config.MaxConcurrentTools)
	}
	if maximum.Load() != int32(config.MaxConcurrentTools) {
		t.Fatalf("maximum active handlers = %d, want %d", maximum.Load(), config.MaxConcurrentTools)
	}
	if workers := countExecuteAllWorkers(t); workers > config.MaxConcurrentTools {
		t.Fatalf("ExecuteAll workers = %d, want <= %d", workers, config.MaxConcurrentTools)
	}
	select {
	case index := <-entered:
		t.Fatalf("handler %d bypassed bounded worker admission", index)
	case <-time.After(25 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	results := <-resultsDone
	for i, result := range results {
		if result.ToolCallID != calls[i].ID || result.Result != i {
			t.Fatalf("result %d = %#v", i, result)
		}
	}
}

func countExecuteAllWorkers(t *testing.T) int {
	t.Helper()
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	var owner string
	for _, stack := range bytes.Split(buf, []byte("\n\n")) {
		if bytes.Contains(stack, []byte("TestToolExecutorWorkerCountAndOrderingAreBounded.func")) &&
			bytes.Contains(stack, []byte("ToolExecutor).ExecuteAll")) {
			owner = strings.Fields(string(stack))[1]
			break
		}
	}
	if owner == "" {
		t.Fatal("ExecuteAll runner goroutine not found")
	}
	return bytes.Count(buf, []byte("ToolExecutor).ExecuteAll in goroutine "+owner))
}

func TestToolExecutorStopsWorkerWhenHandlerOutlivesCancellation(t *testing.T) {
	t.Parallel()
	registry := NewToolRegistry()
	startedHandler := make(chan struct{})
	releaseHandler := make(chan struct{})
	t.Cleanup(func() { close(releaseHandler) })
	registry.Register("tool", types.NewToolDefinition(types.Tool{Name: "tool"}, func(context.Context, map[string]any) (any, error) {
		close(startedHandler)
		<-releaseHandler
		return "late", nil
	}))
	config := DefaultToolSafetyConfig()
	config.MaxConcurrentTools = 1
	config.MaxToolCallsPerRound = 4
	config.ToolTimeout = 10 * time.Millisecond
	executor := NewToolExecutorWithConfig(registry, config)
	t.Cleanup(executor.Stop)

	results := executor.ExecuteAll(context.Background(), []types.ToolCall{
		{ID: "first", Name: "tool"},
		{ID: "second", Name: "tool"},
	})
	<-startedHandler
	if !strings.Contains(results[0].Error, "timed out") {
		t.Fatalf("first result = %#v", results[0])
	}
	if !strings.Contains(results[1].Error, "not started") {
		t.Fatalf("second result = %#v", results[1])
	}
}

func TestOversizedToolBatchDoesNotAllocatePerCall(t *testing.T) {
	t.Parallel()
	config := DefaultToolSafetyConfig()
	executor := NewToolExecutorWithConfig(NewToolRegistry(), config)
	t.Cleanup(executor.Stop)
	calls := make([]types.ToolCall, 65_536)
	var results []types.ToolResult
	benchmark := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			results = executor.ExecuteAll(context.Background(), calls)
		}
	})
	if got := benchmark.AllocedBytesPerOp(); got > 4_096 {
		t.Fatalf("oversized batch allocated %d bytes/op, want <= 4096", got)
	}
	if len(results) != 1 || !strings.Contains(results[0].Error, "exceeds limit") {
		t.Fatalf("results = %#v, want bounded batch error", results)
	}
}

func TestRetainedPermitWaitIsBoundedByQueueTimeout(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	registry.Register("tool", types.NewToolDefinition(types.Tool{Name: "tool"}, func(context.Context, map[string]any) (any, error) {
		started <- struct{}{}
		<-release
		return "late", nil
	}))
	config := DefaultToolSafetyConfig()
	config.MaxConcurrentTools = 1
	config.ToolTimeout = 5 * time.Millisecond
	config.ToolQueueTimeout = 20 * time.Millisecond
	executor := NewToolExecutorWithConfig(registry, config)
	t.Cleanup(executor.Stop)

	first := executor.Execute(context.Background(), types.ToolCall{ID: "first", Name: "tool"})
	<-started
	if !strings.Contains(first.Error, "timed out") {
		t.Fatalf("first result = %#v", first)
	}
	startedAt := time.Now()
	second := executor.Execute(context.Background(), types.ToolCall{ID: "second", Name: "tool"})
	if elapsed := time.Since(startedAt); elapsed > config.ToolQueueTimeout+500*time.Millisecond {
		t.Fatalf("queue wait = %s, want bounded by %s", elapsed, config.ToolQueueTimeout)
	}
	if !strings.Contains(second.Error, "waiting for tool execution permit") {
		t.Fatalf("second result = %#v", second)
	}
	select {
	case <-started:
		t.Fatal("second handler started without a permit")
	default:
	}
}

func TestAutomaticToolLoopRejectsInvalidBatchWithoutContinuation(t *testing.T) {
	t.Parallel()

	executor := NewToolExecutor(NewToolRegistry())
	t.Cleanup(executor.Stop)
	var providerCalls atomic.Int32
	_, err := executor.executeWithTools(context.Background(), types.TextRequest{}, func(context.Context, types.TextRequest) (*types.TextResponse, error) {
		providerCalls.Add(1)
		return &types.TextResponse{ToolCalls: []types.ToolCall{{ID: "", Name: "tool"}}}, nil
	}, 2)
	if err == nil || !strings.Contains(err.Error(), "invalid tool call batch") {
		t.Fatalf("error = %v", err)
	}
	if providerCalls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", providerCalls.Load())
	}
}

type invalidBatchAgentProvider struct {
	*types.BaseProvider
	calls atomic.Int32
}

func (p *invalidBatchAgentProvider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityText, types.CapabilityChat, types.CapabilityFunctions}
}

func (p *invalidBatchAgentProvider) Text(context.Context, types.TextRequest) (*types.TextResponse, error) {
	p.calls.Add(1)
	return &types.TextResponse{ToolCalls: []types.ToolCall{{ID: "", Name: "tool"}}}, nil
}

func TestAgentRejectsInvalidBatchWithoutContinuation(t *testing.T) {
	t.Parallel()

	provider := &invalidBatchAgentProvider{BaseProvider: types.NewBaseProvider("invalid")}
	client := New(
		WithDefaultProvider("invalid"),
		WithCustomProvider("invalid", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
		WithProviderConfig("invalid", types.ProviderConfig{}),
		WithModelValidation(false),
		WithDiscovery(false),
	)
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })
	_, err := client.Agent().
		Model("test").
		AddTool("tool", "test tool", nil, func(context.Context, map[string]any) (any, error) { return nil, nil }).
		Run(context.Background(), "run")
	if err == nil || !strings.Contains(err.Error(), "invalid tool call batch") {
		t.Fatalf("error = %v", err)
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls.Load())
	}
}
