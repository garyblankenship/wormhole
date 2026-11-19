package wormhole

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/internal/schemavalidation"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestAgyreviewC301CooperativeCancellationPreservesErrorAndAdaptiveLatency(t *testing.T) {
	t.Parallel()

	t.Run("cooperative handler error on cancel is not misclassified as permit wait", func(t *testing.T) {
		t.Parallel()

		started := make(chan struct{})
		customErr := errors.New("handler aborted cleanly on cancel")
		registry := NewToolRegistry()
		registry.Register("coop", types.NewToolDefinition(types.Tool{Name: "coop"}, func(ctx context.Context, _ map[string]any) (any, error) {
			close(started)
			<-ctx.Done()
			return nil, customErr
		}))

		config := DefaultToolSafetyConfig()
		config.MaxConcurrentTools = 1
		config.ToolTimeout = 0
		config.EnableAdaptiveConcurrency = true
		config.AdaptiveMinCapacity = 1
		config.AdaptiveMaxCapacity = 2
		config.AdaptiveAdjustmentInterval = time.Hour
		executor := NewToolExecutorWithConfig(registry, config)
		t.Cleanup(executor.Stop)

		ctx, cancel := context.WithCancel(context.Background())
		resultCh := make(chan types.ToolResult, 1)
		go func() {
			resultCh <- executor.Execute(ctx, types.ToolCall{ID: "call-1", Name: "coop"})
		}()

		<-started
		cancel()
		result := <-resultCh

		if strings.Contains(result.Error, "waiting for tool execution permit") {
			t.Fatalf("cooperative cancellation misclassified as permit-wait error: %q", result.Error)
		}
		if !strings.Contains(result.Error, customErr.Error()) && !strings.Contains(result.Error, "timed out or was canceled") {
			t.Fatalf("unexpected error = %q, want handler error or timeout/cancel error", result.Error)
		}

		waitForToolExecutorPermits(t, executor, 0)
		executor.adaptiveLimiter.mu.RLock()
		samples := executor.adaptiveLimiter.sampleCount
		executor.adaptiveLimiter.mu.RUnlock()
		if samples == 0 {
			t.Fatal("adaptive limiter missed latency sample for started handler")
		}
	})

	t.Run("timeout during retry backoff after attempt 0 is not misclassified as permit wait", func(t *testing.T) {
		t.Parallel()

		attempt0Done := make(chan struct{})
		var attempts atomic.Int32
		registry := NewToolRegistry()
		registry.Register("flaky", types.NewToolDefinition(types.Tool{Name: "flaky"}, func(context.Context, map[string]any) (any, error) {
			if attempts.Add(1) == 1 {
				close(attempt0Done)
			}
			return nil, errors.New("transient failure")
		}))

		config := DefaultToolSafetyConfig()
		config.MaxConcurrentTools = 1
		config.ToolTimeout = 0
		config.MaxRetriesPerTool = 3
		config.EnableAdaptiveConcurrency = true
		config.AdaptiveMinCapacity = 1
		config.AdaptiveMaxCapacity = 2
		config.AdaptiveAdjustmentInterval = time.Hour
		executor := NewToolExecutorWithConfig(registry, config)
		t.Cleanup(executor.Stop)

		ctx, cancel := context.WithCancel(context.Background())
		resultCh := make(chan types.ToolResult, 1)
		go func() {
			resultCh <- executor.Execute(ctx, types.ToolCall{ID: "call-retry", Name: "flaky"})
		}()

		<-attempt0Done
		// Give RetryExecutor a moment to enter backoff timer after callToolHandler's defer ran.
		time.Sleep(15 * time.Millisecond)
		cancel()
		result := <-resultCh

		if strings.Contains(result.Error, "waiting for tool execution permit") {
			t.Fatalf("retry backoff cancellation misclassified as permit-wait error: %q", result.Error)
		}
		if result.Error == "" {
			t.Fatal("expected cancellation error, got empty error")
		}

		waitForToolExecutorPermits(t, executor, 0)
		executor.adaptiveLimiter.mu.RLock()
		samples := executor.adaptiveLimiter.sampleCount
		executor.adaptiveLimiter.mu.RUnlock()
		if samples == 0 {
			t.Fatal("adaptive limiter missed latency sample after retry backoff cancellation")
		}
	})
}

type c3CircuitBreakerMockProvider struct {
	*types.BaseProvider
	mu            sync.Mutex
	seenToolError []string
}

func (p *c3CircuitBreakerMockProvider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{types.CapabilityText, types.CapabilityChat, types.CapabilityFunctions}
}

func (p *c3CircuitBreakerMockProvider) Text(_ context.Context, req types.TextRequest) (*types.TextResponse, error) {
	// If the last message is a tool result, record its error and finish the turn.
	if len(req.Messages) > 0 {
		if toolMsg, ok := req.Messages[len(req.Messages)-1].(*types.ToolResultMessage); ok {
			p.mu.Lock()
			p.seenToolError = append(p.seenToolError, toolMsg.Error)
			p.mu.Unlock()
			return &types.TextResponse{Text: "done"}, nil
		}
	}
	return &types.TextResponse{
		ToolCalls: []types.ToolCall{{ID: "call-1", Name: "failing_tool", Arguments: map[string]any{}}},
	}, nil
}

func TestAgyreviewC302ClientCircuitBreakersPersistAcrossCalls(t *testing.T) {
	t.Parallel()

	provider := &c3CircuitBreakerMockProvider{BaseProvider: types.NewBaseProvider("mock")}
	safety := DefaultToolSafetyConfig()
	safety.EnableCircuitBreaker = true
	safety.CircuitBreakerThreshold = 1
	safety.CircuitBreakerResetTimeout = time.Hour

	client := New(
		WithDefaultProvider("mock"),
		WithCustomProvider("mock", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
		WithProviderConfig("mock", types.ProviderConfig{}),
		WithToolSafetyConfig(safety),
		WithModelValidation(false),
		WithDiscovery(false),
	)
	t.Cleanup(func() { _ = client.Shutdown(context.Background()) })

	var handlerCalls atomic.Int32
	client.RegisterTool("failing_tool", "always fails", map[string]any{"type": "object"}, func(context.Context, map[string]any) (any, error) {
		handlerCalls.Add(1)
		return nil, errors.New("boom")
	})

	// First Text().Generate() call triggers 1 failure, tripping the client-level circuit breaker.
	if _, err := client.Text().Model("test-model").Prompt("first").Generate(context.Background()); err != nil {
		t.Fatalf("first Text().Generate() error = %v", err)
	}
	if got := handlerCalls.Load(); got != 1 {
		t.Fatalf("handlerCalls after first Text() = %d, want 1", got)
	}

	// Second Text().Generate() call must observe the tripped breaker and NOT invoke the handler.
	if _, err := client.Text().Model("test-model").Prompt("second").Generate(context.Background()); err != nil {
		t.Fatalf("second Text().Generate() error = %v", err)
	}
	if got := handlerCalls.Load(); got != 1 {
		t.Fatalf("handlerCalls after second Text() = %d, want 1 (breaker should stay open)", got)
	}

	// Subsequent Agent().Run() call on the same client must also observe the tripped breaker.
	if _, err := client.Agent().Model("test-model").Run(context.Background(), "third"); err != nil {
		t.Fatalf("Agent().Run() error = %v", err)
	}
	if got := handlerCalls.Load(); got != 1 {
		t.Fatalf("handlerCalls after Agent().Run() = %d, want 1 (breaker should persist into Agent)", got)
	}

	provider.mu.Lock()
	errs := append([]string(nil), provider.seenToolError...)
	provider.mu.Unlock()
	if len(errs) != 3 {
		t.Fatalf("seenToolError len = %d, want 3 (%v)", len(errs), errs)
	}
	if !strings.Contains(errs[0], "boom") {
		t.Fatalf("first tool error = %q, want boom", errs[0])
	}
	if !strings.Contains(errs[1], "circuit breaker tripped") {
		t.Fatalf("second tool error = %q, want circuit breaker tripped", errs[1])
	}
	if !strings.Contains(errs[2], "circuit breaker tripped") {
		t.Fatalf("third tool error = %q, want circuit breaker tripped", errs[2])
	}

	// Direct NewToolExecutorWithConfig callers remain isolated from each other.
	reg := NewToolRegistry()
	var directCalls atomic.Int32
	reg.Register("failing_tool", types.NewToolDefinition(types.Tool{Name: "failing_tool"}, func(context.Context, map[string]any) (any, error) {
		directCalls.Add(1)
		return nil, errors.New("direct boom")
	}))
	execA := NewToolExecutorWithConfig(reg, safety)
	execB := NewToolExecutorWithConfig(reg, safety)
	t.Cleanup(execA.Stop)
	t.Cleanup(execB.Stop)

	_ = execA.Execute(context.Background(), types.ToolCall{ID: "a1", Name: "failing_tool"})
	resA2 := execA.Execute(context.Background(), types.ToolCall{ID: "a2", Name: "failing_tool"})
	if !strings.Contains(resA2.Error, "circuit breaker tripped") {
		t.Fatalf("execA second call = %q, want circuit breaker tripped", resA2.Error)
	}
	resB1 := execB.Execute(context.Background(), types.ToolCall{ID: "b1", Name: "failing_tool"})
	if !strings.Contains(resB1.Error, "direct boom") {
		t.Fatalf("execB first call = %q, want isolated breaker to execute handler", resB1.Error)
	}
	if got := directCalls.Load(); got != 2 {
		t.Fatalf("directCalls = %d, want 2", got)
	}
}

func TestAgyreviewC303ToolTagPreservesCommasInPatternAndDefault(t *testing.T) {
	t.Parallel()

	type TagArgs struct {
		Code           string `json:"code" tool:"pattern=^[a-z]{1,5}$"`
		ReqCode        string `json:"req_code" tool:"required,pattern=^[a-z]{1,5}$"`
		DefaultCSV     string `json:"default_csv" tool:"default=a,b,c"`
		ReqDefaultCSV  string `json:"req_default_csv" tool:"required,default=hello, world"`
		PatternWithTag string `json:"pattern_enum" tool:"pattern=^enum=[a-z]{1,3}$"`
		PatternSemi    string `json:"pattern_semi" tool:"pattern=^[0-9]{2,4}$;required"`
	}

	schema, err := SchemaFromStruct(TagArgs{})
	if err != nil {
		t.Fatalf("SchemaFromStruct error = %v", err)
	}

	props := schema["properties"].(map[string]any)
	if got := props["code"].(map[string]any)["pattern"]; got != "^[a-z]{1,5}$" {
		t.Fatalf("code pattern = %q, want %q", got, "^[a-z]{1,5}$")
	}
	if got := props["req_code"].(map[string]any)["pattern"]; got != "^[a-z]{1,5}$" {
		t.Fatalf("req_code pattern = %q, want %q", got, "^[a-z]{1,5}$")
	}
	if got := props["default_csv"].(map[string]any)["default"]; got != "a,b,c" {
		t.Fatalf("default_csv default = %q, want %q", got, "a,b,c")
	}
	if got := props["req_default_csv"].(map[string]any)["default"]; got != "hello, world" {
		t.Fatalf("req_default_csv default = %q, want %q", got, "hello, world")
	}
	if got := props["pattern_enum"].(map[string]any)["pattern"]; got != "^enum=[a-z]{1,3}$" {
		t.Fatalf("pattern_enum pattern = %q, want %q", got, "^enum=[a-z]{1,3}$")
	}
	if got := props["pattern_semi"].(map[string]any)["pattern"]; got != "^[0-9]{2,4}$" {
		t.Fatalf("pattern_semi pattern = %q, want %q", got, "^[0-9]{2,4}$")
	}

	required := schema["required"].([]string)
	wantRequired := map[string]bool{"req_code": false, "req_default_csv": false, "pattern_semi": false}
	for _, r := range required {
		if _, ok := wantRequired[r]; ok {
			wantRequired[r] = true
		}
	}
	for name, found := range wantRequired {
		if !found {
			t.Fatalf("required missing %q; got %v", name, required)
		}
	}

	// Verify the generated regex actually validates via schemavalidation.
	if err := schemavalidation.ValidateAgainstSchema(map[string]any{
		"code":            "abc",
		"req_code":        "hello",
		"req_default_csv": "x",
		"pattern_semi":    "123",
	}, schema); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	if err := schemavalidation.ValidateAgainstSchema(map[string]any{
		"code":            "abcdef",
		"req_code":        "hello",
		"req_default_csv": "x",
		"pattern_semi":    "123",
	}, schema); err == nil {
		t.Fatal("over-length pattern payload unexpectedly accepted")
	}
}

func TestAgyreviewC304AdaptiveExecuteAllScalesAboveInitialMaxConcurrentTools(t *testing.T) {
	t.Parallel()

	var active atomic.Int32
	var peak atomic.Int32
	allEntered := make(chan struct{})
	release := make(chan struct{})
	var closeOnce sync.Once
	t.Cleanup(func() { closeOnce.Do(func() { close(release) }) })

	const targetConcurrency = 4
	registry := NewToolRegistry()
	registry.Register("work", types.NewToolDefinition(types.Tool{Name: "work"}, func(context.Context, map[string]any) (any, error) {
		cur := active.Add(1)
		for {
			old := peak.Load()
			if cur <= old || peak.CompareAndSwap(old, cur) {
				break
			}
		}
		if cur == targetConcurrency {
			close(allEntered)
		}
		<-release
		active.Add(-1)
		return "ok", nil
	}))

	config := DefaultToolSafetyConfig()
	config.MaxConcurrentTools = 1 // InitialCapacity = 1
	config.EnableAdaptiveConcurrency = true
	config.AdaptiveMinCapacity = 1
	config.AdaptiveMaxCapacity = targetConcurrency
	config.AdaptiveTargetLatency = time.Second
	config.AdaptiveAdjustmentInterval = time.Hour
	executor := NewToolExecutorWithConfig(registry, config)
	t.Cleanup(executor.Stop)

	// Scale the adaptive limiter up from InitialCapacity (1) to AdaptiveMaxCapacity (4).
	for range targetConcurrency - 1 {
		executor.adaptiveLimiter.RecordLatency(time.Millisecond)
		executor.adaptiveLimiter.adjustCapacity()
	}

	calls := make([]types.ToolCall, targetConcurrency)
	for i := range calls {
		calls[i] = types.ToolCall{ID: string(rune('a' + i)), Name: "work"}
	}

	done := make(chan []types.ToolResult, 1)
	go func() {
		done <- executor.ExecuteAll(context.Background(), calls)
	}()

	select {
	case <-allEntered:
	case <-time.After(2 * time.Second):
		t.Fatalf("ExecuteAll did not scale workers above MaxConcurrentTools=1; peak active = %d, want %d", peak.Load(), targetConcurrency)
	}
	closeOnce.Do(func() { close(release) })
	results := <-done
	if len(results) != targetConcurrency {
		t.Fatalf("results len = %d, want %d", len(results), targetConcurrency)
	}
	if got := peak.Load(); got != targetConcurrency {
		t.Fatalf("peak concurrency = %d, want %d", got, targetConcurrency)
	}
}

func TestAgyreviewC305ToolRegistryRegisterNilDefinitionDoesNotPanic(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry()
	registry.Register("nil_tool", nil)
	if registry.Has("nil_tool") {
		t.Fatal("nil tool definition should not be stored in registry")
	}
	if registry.Count() != 0 {
		t.Fatalf("Count() = %d, want 0", registry.Count())
	}
}

func TestAgyreviewC306UnexportedAnonymousTaggedStructMatchesEncodingJSON(t *testing.T) {
	t.Parallel()

	type unexportedInner struct {
		Value string `json:"value"`
	}
	type outer struct {
		unexportedInner `json:"nested"`
	}

	schema, err := SchemaFromStruct(outer{})
	if err != nil {
		t.Fatalf("SchemaFromStruct error = %v", err)
	}
	props := schema["properties"].(map[string]any)
	if _, ok := props["nested"]; !ok {
		t.Fatalf("expected nested property to match encoding/json, got %v", props)
	}

	var decoded outer
	if err := json.Unmarshal([]byte(`{"nested":{"value":"ok"}}`), &decoded); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}
	if decoded.Value != "ok" {
		t.Fatalf("decoded.Value = %q, want ok", decoded.Value)
	}
}

func TestAgyreviewC307ExecuteWithToolsDoesNotMutateCallerMessagesBackingArray(t *testing.T) {
	t.Parallel()

	registry := NewToolRegistry()
	registry.Register("echo", types.NewToolDefinition(types.Tool{Name: "echo"}, func(context.Context, map[string]any) (any, error) {
		return "echoed", nil
	}))
	executor := NewToolExecutor(registry)
	t.Cleanup(executor.Stop)

	sentinel := types.NewUserMessage("sentinel-must-not-be-overwritten")
	backing := make([]types.Message, 2, 8)
	backing[0] = types.NewUserMessage("initial")
	backing[1] = sentinel
	callerMessages := backing[:1] // len=1, cap=8; backing[1] is in spare capacity

	var turn int
	resp, err := executor.executeWithTools(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "test-model"},
		Messages:    callerMessages,
	}, func(_ context.Context, _ types.TextRequest) (*types.TextResponse, error) {
		turn++
		if turn == 1 {
			return &types.TextResponse{
				ToolCalls: []types.ToolCall{{ID: "call-1", Name: "echo", Arguments: map[string]any{}}},
			}, nil
		}
		return &types.TextResponse{Text: "final"}, nil
	}, 4)
	if err != nil {
		t.Fatalf("executeWithTools error = %v", err)
	}
	if resp.Text != "final" {
		t.Fatalf("resp.Text = %q, want final", resp.Text)
	}
	if backing[1] != sentinel {
		t.Fatalf("caller Messages backing array was mutated: backing[1] = %#v, want sentinel", backing[1])
	}
}
