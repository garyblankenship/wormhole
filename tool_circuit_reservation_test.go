package wormhole

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func expireProbeBreaker(cb *SimpleCircuitBreaker) {
	cb.RecordFailure()
	cb.mu.Lock()
	cb.tripExpiry = time.Now().Add(-time.Second)
	cb.mu.Unlock()
}

func TestCircuitReservationAbandonedPermitWait(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		config := DefaultToolSafetyConfig()
		config.EnableCircuitBreaker = true
		config.CircuitBreakerThreshold = 1
		config.MaxConcurrentTools = 1
		executor := remediationToolExecutor(t, config, map[string]types.ToolHandler{"tool": func(context.Context, map[string]any) (any, error) { return true, nil }})
		cb := executor.breakerForTool("tool")
		expireProbeBreaker(cb)
		if !executor.limiter.Acquire(context.Background()) {
			t.Fatal("occupy permit")
		}
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		done := make(chan types.ToolResult, 1)
		go func() { done <- executor.Execute(ctx, types.ToolCall{Name: "tool"}) }()
		synctest.Wait()
		cb.mu.RLock()
		reserved := cb.halfOpenCalls
		cb.mu.RUnlock()
		if reserved != 1 {
			t.Fatalf("reserved=%d", reserved)
		}
		cancel()
		if result := <-done; result.Error == "" {
			t.Fatal("canceled request succeeded")
		}
		executor.limiter.Release()
		if result := executor.Execute(context.Background(), types.ToolCall{Name: "tool"}); result.Error != "" {
			t.Fatalf("next probe: %s", result.Error)
		}
	})
}

type cancelOnPermitContext struct {
	context.Context
	cancel   context.CancelFunc
	limiter  *ConcurrencyLimiter
	canceled atomic.Bool
}

func (c *cancelOnPermitContext) Err() error {
	if c.limiter.InUse() != 0 && c.canceled.CompareAndSwap(false, true) {
		c.cancel()
	}
	return c.Context.Err()
}

func TestCircuitReservationAbandonedAfterPermit(t *testing.T) {
	t.Parallel()
	config := DefaultToolSafetyConfig()
	config.EnableCircuitBreaker = true
	config.CircuitBreakerThreshold = 1
	config.MaxConcurrentTools = 1
	config.ToolTimeout = 0
	var calls atomic.Int32
	executor := remediationToolExecutor(t, config, map[string]types.ToolHandler{"tool": func(context.Context, map[string]any) (any, error) { calls.Add(1); return true, nil }})
	expireProbeBreaker(executor.breakerForTool("tool"))
	base, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctx := &cancelOnPermitContext{Context: base, cancel: cancel, limiter: executor.limiter}
	if result := executor.Execute(ctx, types.ToolCall{Name: "tool"}); result.Error == "" {
		t.Fatal("canceled request succeeded")
	}
	if calls.Load() != 0 || executor.limiter.InUse() != 0 {
		t.Fatalf("calls=%d permits=%d", calls.Load(), executor.limiter.InUse())
	}
	if result := executor.Execute(context.Background(), types.ToolCall{Name: "tool"}); result.Error != "" {
		t.Fatalf("next probe: %s", result.Error)
	}
}

func TestCircuitReservationAbandonedArguments(t *testing.T) {
	t.Parallel()
	for _, malformed := range []bool{true, false} {
		t.Run(map[bool]string{true: "malformed", false: "schema"}[malformed], func(t *testing.T) {
			t.Parallel()
			config := DefaultToolSafetyConfig()
			config.EnableCircuitBreaker = true
			config.CircuitBreakerThreshold = 1
			config.EnableInputValidation = true
			executor := remediationToolExecutor(t, config, map[string]types.ToolHandler{"tool": func(context.Context, map[string]any) (any, error) { return true, nil }})
			executor.registry.getStored("tool").Tool.InputSchema = map[string]any{"type": "object", "required": []string{"value"}, "properties": map[string]any{"value": map[string]any{"type": "string"}}}
			cb := executor.breakerForTool("tool")
			expireProbeBreaker(cb)
			rejected := executor.Execute(context.Background(), types.ToolCall{Name: "tool", ArgsInvalid: malformed})
			if rejected.Error == "" {
				t.Fatal("arguments accepted")
			}
			cb.mu.RLock()
			state, slots := cb.state, cb.halfOpenCalls
			cb.mu.RUnlock()
			expectedState := stateHalfOpen
			if malformed {
				expectedState = stateOpen // Normalization rejects before reserving.
			}
			if state != expectedState || slots != 0 {
				t.Fatalf("state=%d slots=%d", state, slots)
			}
			result := executor.Execute(context.Background(), types.ToolCall{Name: "tool", Arguments: map[string]any{"value": "ok"}})
			if result.Error != "" {
				t.Fatalf("next probe: %s", result.Error)
			}
		})
	}
}

func TestCircuitReservationStaleAndOnce(t *testing.T) {
	t.Parallel()
	cb := NewSimpleCircuitBreaker(1, time.Minute)
	cb.maxHalfOpenCalls = 2
	expireProbeBreaker(cb)
	first, ok := cb.reserve()
	if !ok {
		t.Fatal("first denied")
	}
	stale, ok := cb.reserve()
	if !ok {
		t.Fatal("second denied")
	}
	first.settle(true, false)
	cb.mu.Lock()
	cb.tripExpiry = time.Now().Add(-time.Second)
	cb.mu.Unlock()
	current, ok := cb.reserve()
	if !ok {
		t.Fatal("new generation denied")
	}
	stale.settle(true, true)
	stale.settle(false, false)
	if cb.state != stateHalfOpen || cb.halfOpenCalls != 1 {
		t.Fatalf("stale changed current generation: %#v", cb)
	}
	current.settle(false, false)
	next, ok := cb.reserve()
	if !ok {
		t.Fatal("neutral settlement did not release")
	}
	current.settle(true, true)
	if cb.state != stateHalfOpen || cb.halfOpenCalls != 1 {
		t.Fatal("reservation settled twice")
	}
	next.settle(true, true)
	if cb.state != stateClosed {
		t.Fatal("probe success did not close")
	}
}

func TestCircuitReservationPanicRecordsFailure(t *testing.T) {
	t.Parallel()
	for _, halfOpen := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "half-open"}[halfOpen], func(t *testing.T) {
			t.Parallel()
			config := DefaultToolSafetyConfig()
			config.EnableCircuitBreaker = true
			config.CircuitBreakerThreshold = 1
			config.MaxRetriesPerTool = 0
			var calls atomic.Int32
			executor := remediationToolExecutor(t, config, map[string]types.ToolHandler{
				"panic": func(context.Context, map[string]any) (any, error) {
					calls.Add(1)
					panic("handler failure")
				},
			})
			cb := executor.breakerForTool("panic")
			if halfOpen {
				expireProbeBreaker(cb)
			}
			result := executor.Execute(context.Background(), types.ToolCall{Name: "panic"})
			if !strings.Contains(result.Error, "tool handler panicked: handler failure") {
				t.Fatalf("panic result: %#v", result)
			}
			cb.mu.RLock()
			state, failures := cb.state, cb.failureCount
			cb.mu.RUnlock()
			if state != stateOpen || failures != 1 {
				t.Fatalf("panic accounting: state=%d failures=%d", state, failures)
			}
			blocked := executor.Execute(context.Background(), types.ToolCall{Name: "panic"})
			if !strings.Contains(blocked.Error, "circuit breaker tripped") || calls.Load() != 1 {
				t.Fatalf("next call=%#v handler calls=%d", blocked, calls.Load())
			}
		})
	}
}
