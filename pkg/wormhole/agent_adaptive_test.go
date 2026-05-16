package wormhole

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestAgentBuilderConfigurationAndRun(t *testing.T) {
	t.Parallel()

	provider := &mockToolProvider{
		responses: []*types.TextResponse{
			{
				ID:           "final",
				Model:        "mock-model",
				Text:         "done",
				FinishReason: types.FinishReasonStop,
			},
		},
	}
	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) {
			return provider, nil
		}),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithDiscovery(false),
	)

	var observed []StepEvent
	builder := client.Agent().
		Using("mock").
		Model("mock-model").
		System("system").
		MaxSteps(3).
		Temperature(0.2).
		MaxTokens(64).
		OnStep(func(e StepEvent) {
			observed = append(observed, e)
		}).
		AddTool("lookup", "Lookup data", map[string]any{"type": "object"}, func(context.Context, map[string]any) (any, error) {
			return "ok", nil
		})

	if builder.provider != "mock" || builder.model != "mock-model" || builder.systemPrompt != "system" {
		t.Fatalf("agent builder identity = %#v", builder)
	}
	if *builder.temperature != 0.2 || *builder.maxTokens != 64 || builder.maxSteps != 3 {
		t.Fatalf("agent builder generation config = %#v", builder)
	}

	result, err := builder.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.TotalSteps != 1 || result.Response.Text != "done" {
		t.Fatalf("agent result = %#v", result)
	}
	if len(observed) != 1 || !observed[0].Done {
		t.Fatalf("observed steps = %#v", observed)
	}
}

func TestAgentBuilderValidationAndToolMerge(t *testing.T) {
	t.Parallel()

	client := New(WithDefaultProvider("mock"), WithDiscovery(false))
	builder := client.Agent()
	if _, err := builder.Run(context.Background(), "hello"); err == nil {
		t.Fatal("Run without model returned nil error")
	}

	builder.Model("mock-model")
	if _, err := builder.Run(context.Background(), "hello"); err == nil {
		t.Fatal("Run without tools returned nil error")
	}

	client.RegisterTool("shared", "global", map[string]any{"type": "object"}, func(context.Context, map[string]any) (any, error) {
		return "global", nil
	})
	builder.AddTool("shared", "agent", map[string]any{"type": "object"}, func(context.Context, map[string]any) (any, error) {
		return "agent", nil
	})
	merged := builder.mergeTools()
	if merged.Count() != 1 {
		t.Fatalf("merged tool count = %d, want 1", merged.Count())
	}
	if got := merged.Get("shared").Tool.Description; got != "agent" {
		t.Fatalf("merged tool description = %q, want agent", got)
	}
}

func TestAgentAddTool(t *testing.T) {
	t.Parallel()

	type searchArgs struct {
		Query string `json:"query" tool:"required" desc:"Search query"`
	}

	builder := New(WithDiscovery(false)).Agent()
	err := AgentAddTool(builder, "search", "Search", func(ctx context.Context, args searchArgs) (string, error) {
		return args.Query, nil
	})
	if err != nil {
		t.Fatalf("AgentAddTool returned error: %v", err)
	}
	def := builder.tools.Get("search")
	if def == nil {
		t.Fatal("search tool was not registered")
	}
	result, err := def.Handler(context.Background(), map[string]any{"query": "go"})
	if err != nil {
		t.Fatalf("tool handler returned error: %v", err)
	}
	if result != "go" {
		t.Fatalf("tool result = %#v, want go", result)
	}
}

func TestAdaptiveLimiter(t *testing.T) {
	t.Parallel()

	config := AdaptiveConfig{
		TargetLatency:      10 * time.Millisecond,
		MinCapacity:        1,
		MaxCapacity:        3,
		InitialCapacity:    5,
		AdjustmentInterval: time.Hour,
		LatencyWindowSize:  2,
	}
	limiter := NewAdaptiveLimiter(config)
	defer limiter.Stop()

	if limiter.limiter.Capacity() != 3 {
		t.Fatalf("initial capacity = %d, want clamp to 3", limiter.limiter.Capacity())
	}
	release, ok := limiter.AcquireToken(context.Background())
	if !ok {
		t.Fatal("AcquireToken failed")
	}
	release()
	if !limiter.Acquire(context.Background()) {
		t.Fatal("Acquire failed")
	}
	limiter.Release()

	limiter.RecordLatency(20 * time.Millisecond)
	limiter.adjustCapacity()
	if limiter.limiter.Capacity() != 2 {
		t.Fatalf("capacity after high latency = %d, want 2", limiter.limiter.Capacity())
	}

	limiter.RecordLatency(time.Millisecond)
	limiter.adjustCapacity()
	if limiter.limiter.Capacity() != 3 {
		t.Fatalf("capacity after low latency = %d, want 3", limiter.limiter.Capacity())
	}

	var releases []func()
	for range limiter.limiter.Capacity() {
		release, ok := limiter.AcquireToken(context.Background())
		if !ok {
			t.Fatal("AcquireToken failed while filling limiter")
		}
		releases = append(releases, release)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if release, ok := limiter.AcquireToken(ctx); ok || release != nil {
		t.Fatalf("AcquireToken canceled with full limiter returned release nil=%t ok=%t, want nil false", release == nil, ok)
	}
	for _, release := range releases {
		release()
	}
}

func TestAdaptiveCapacityHelpers(t *testing.T) {
	t.Parallel()

	if got := splitLabelKey("provider:model:text:timeout"); len(got) != 4 || got[0] != "provider" || got[3] != "timeout" {
		t.Fatalf("splitLabelKey = %#v", got)
	}

	observer := &metricsObserver{config: EnhancedAdaptiveConfig{ErrorRateThreshold: 0.1}}
	observer.enhanceControlWithMetrics(nil, map[string]interface{}{
		"errors":   int64(2),
		"requests": int64(10),
	})
	observer.enhanceControlWithMetrics(nil, map[string]interface{}{
		"errors":   int64(0),
		"requests": int64(0),
	})
}

func TestToolSafetyConfigValidationAndHelpers(t *testing.T) {
	t.Parallel()

	config := ToolSafetyConfig{
		MaxConcurrentTools:         -1,
		ToolTimeout:                -1,
		MaxRetriesPerTool:          -1,
		CircuitBreakerThreshold:    0,
		CircuitBreakerResetTimeout: -1,
		AdaptiveMinCapacity:        0,
		AdaptiveMaxCapacity:        0,
		AdaptiveTargetLatency:      0,
		AdaptiveAdjustmentInterval: 0,
		AdaptiveLatencyWindowSize:  0,
		MaxMemoryMB:                -1,
		MaxCPUTime:                 -1,
		MaxToolOutputSize:          0,
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate normalized config returned error: %v", err)
	}
	if !config.IsUnlimitedConcurrency() {
		t.Fatal("expected unlimited concurrency after normalization")
	}
	if config.HasTimeout() || config.HasMemoryLimit() || config.HasCPULimit() {
		t.Fatal("unexpected timeout/memory/cpu limit after normalization")
	}
	if !config.HasOutputSizeLimit() {
		t.Fatal("expected default output size limit")
	}

	adaptive := config.ToAdaptiveConfig()
	if adaptive.MinCapacity != 1 || adaptive.MaxCapacity != 1 || adaptive.InitialCapacity != 0 {
		t.Fatalf("adaptive config = %#v", adaptive)
	}

	unsupported := DefaultToolSafetyConfig()
	unsupported.MaxMemoryMB = 1
	if err := unsupported.Validate(); err == nil {
		t.Fatal("Validate with MaxMemoryMB returned nil error")
	}
	unsupported = DefaultToolSafetyConfig()
	unsupported.MaxCPUTime = time.Second
	if err := unsupported.Validate(); err == nil {
		t.Fatal("Validate with MaxCPUTime returned nil error")
	}
	unsupported = DefaultToolSafetyConfig()
	unsupported.EnableResourceIsolation = true
	if err := unsupported.Validate(); err == nil {
		t.Fatal("Validate with EnableResourceIsolation returned nil error")
	}
}

func TestRetryExecutorContextCancellation(t *testing.T) {
	t.Parallel()

	executor := NewRetryExecutor(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := executor.ExecuteWithRetry(ctx, func(context.Context) error {
		return errors.New("retry")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecuteWithRetry canceled error = %v, want context.Canceled", err)
	}
}
