package wormhole

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func remediationToolExecutor(t *testing.T, config ToolSafetyConfig, handlers map[string]types.ToolHandler) *ToolExecutor {
	t.Helper()
	registry := NewToolRegistry()
	for name, handler := range handlers {
		registry.Register(name, types.NewToolDefinition(types.Tool{Name: name}, handler))
	}
	executor := NewToolExecutorWithConfig(registry, config)
	t.Cleanup(executor.Stop)
	return executor
}

func TestRemediationC202C207RawJSONToolMessages(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value any
		want  string
	}{
		{"hello", `"hello"`}, {nil, "null"}, {map[string]any{"ok": true}, `{"ok":true}`},
	} {
		message := buildToolResultMessage(types.ToolResult{ToolCallID: "id", Name: "tool", Result: tc.value})
		if message.Content != tc.want || message.Error != "" || message.FunctionName != "tool" {
			t.Fatalf("message: %#v", message)
		}
	}
	for _, result := range []types.ToolResult{{ToolCallID: "id", Error: "failed"}, {ToolCallID: "id", Result: make(chan int)}} {
		message := buildToolResultMessage(result)
		if message.Error == "" || !strings.Contains(message.Content, "failed") {
			t.Fatalf("failure: %#v", message)
		}
	}
}

func TestRemediationC205C2R01NormalizedToolBreakerIsolation(t *testing.T) {
	t.Parallel()
	config := DefaultToolSafetyConfig()
	config.EnableCircuitBreaker = true
	config.CircuitBreakerThreshold = 1
	executor := remediationToolExecutor(t, config, map[string]types.ToolHandler{
		"bad":  func(context.Context, map[string]any) (any, error) { return nil, errors.New("bad handler") },
		"good": func(context.Context, map[string]any) (any, error) { return true, nil },
	})
	bad := types.ToolCall{ID: "bad", Function: &types.ToolCallFunction{Name: "bad", Arguments: "{}"}}
	first := executor.Execute(context.Background(), bad)
	if first.Name != "bad" || first.Error == "" {
		t.Fatalf("bad result: %#v", first)
	}
	second := executor.Execute(context.Background(), bad)
	if !strings.Contains(second.Error, "circuit breaker tripped") {
		t.Fatalf("breaker: %#v", second)
	}
	good := executor.Execute(context.Background(), types.ToolCall{ID: "good", Function: &types.ToolCallFunction{Name: "good", Arguments: "{}"}})
	if good.Name != "good" || good.Error != "" {
		t.Fatalf("isolated result: %#v", good)
	}
}

func TestRemediationC2R02SerializationAndByteLimits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		output any
		limit  int
		failed bool
	}{
		{"exact string", "abc", 5, false}, {"over string", "abc", 4, true}, {"exact null", nil, 4, false}, {"over null", nil, 3, true},
		{"serialization", make(chan int), -1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			config := DefaultToolSafetyConfig()
			config.MaxToolOutputSize = tc.limit
			executor := remediationToolExecutor(t, config, map[string]types.ToolHandler{"tool": func(context.Context, map[string]any) (any, error) { return tc.output, nil }})
			call := types.ToolCall{ID: "id", Name: "tool"}
			single := executor.Execute(context.Background(), call)
			batch := executor.ExecuteAll(context.Background(), []types.ToolCall{call})
			if (single.Error != "") != tc.failed || len(batch) != 1 || (batch[0].Error != "") != tc.failed {
				t.Fatalf("single=%#v batch=%#v", single, batch)
			}
		})
	}
}

func TestRemediationC203BatchContinuesAfterTimeout(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		config := DefaultToolSafetyConfig()
		config.MaxConcurrentTools = 1
		config.ToolTimeout = 10 * time.Millisecond
		config.ToolQueueTimeout = time.Second
		var active atomic.Int32
		executor := remediationToolExecutor(t, config, map[string]types.ToolHandler{
			"slow": func(context.Context, map[string]any) (any, error) {
				active.Add(1)
				<-time.After(40 * time.Millisecond)
				active.Add(-1)
				return true, nil
			},
			"next": func(context.Context, map[string]any) (any, error) {
				if active.Load() != 0 {
					return nil, errors.New("admitted alongside timed-out handler")
				}
				return true, nil
			},
		})
		results := executor.ExecuteAll(context.Background(), []types.ToolCall{{ID: "slow", Name: "slow"}, {ID: "next", Name: "next"}})
		if len(results) != 2 || results[0].Error == "" || results[1].Error != "" || results[1].Result != true {
			t.Fatalf("results: %#v", results)
		}
	})
}

func TestRemediationC206CanceledRetryBackoffHasNoRunningHandler(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		config := DefaultToolSafetyConfig()
		config.ToolTimeout = 0
		config.MaxRetriesPerTool = 3
		config.MaxConcurrentTools = 1
		var attempts atomic.Int32
		executor := remediationToolExecutor(t, config, map[string]types.ToolHandler{"tool": func(context.Context, map[string]any) (any, error) { attempts.Add(1); return nil, errors.New("retry") }})
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		type outcome struct {
			result  types.ToolResult
			running bool
		}
		done := make(chan outcome, 1)
		go func() {
			r, running := executor.execute(ctx, types.ToolCall{ID: "id", Name: "tool"})
			done <- outcome{r, running}
		}()
		synctest.Wait()
		cancel()
		result := <-done
		synctest.Wait()
		if result.running || result.result.Error == "" || attempts.Load() != 1 || executor.limiter.InUse() != 0 {
			t.Fatalf("outcome=%#v attempts=%d inUse=%d", result, attempts.Load(), executor.limiter.InUse())
		}
	})
}
