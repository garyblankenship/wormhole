package wormhole

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestToolExecutor_UnsupportedSafetySettingsFailFast(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*ToolSafetyConfig)
	}{
		{
			name: "memory limit",
			configure: func(config *ToolSafetyConfig) {
				config.MaxMemoryMB = 128
			},
		},
		{
			name: "CPU limit",
			configure: func(config *ToolSafetyConfig) {
				config.MaxCPUTime = time.Second
			},
		},
		{
			name: "resource isolation",
			configure: func(config *ToolSafetyConfig) {
				config.EnableResourceIsolation = true
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			called := false
			config := DefaultToolSafetyConfig()
			test.configure(&config)
			client := New(WithToolSafetyConfig(config), WithDiscovery(false))
			client.RegisterTool(
				"test_tool",
				"test unsupported safety configuration",
				map[string]any{},
				func(context.Context, map[string]any) (any, error) {
					called = true
					return map[string]any{"ok": true}, nil
				},
			)

			executor := client.newToolExecutor(client.toolRegistry)
			result := executor.Execute(context.Background(), types.ToolCall{
				ID:        "call_1",
				Name:      "test_tool",
				Arguments: map[string]any{},
			})

			assert.Contains(t, result.Error, "unsupported tool safety settings")
			assert.False(t, called, "handler started for unsupported setting")
			assert.NoError(t, client.Close(), "client cleanup failed for unsupported setting")
		})
	}
}

func TestToolExecutor_ToolTimeoutExcludesConcurrencyQueueWait(t *testing.T) {
	registry := NewToolRegistry()
	firstStarted := make(chan struct{}, 1)
	registry.Register("slow_tool", types.NewToolDefinition(types.Tool{
		Type:        "function",
		Name:        "slow_tool",
		InputSchema: map[string]any{},
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		select {
		case firstStarted <- struct{}{}:
		default:
		}

		timer := time.NewTimer(75 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}))

	config := DefaultToolSafetyConfig()
	config.MaxConcurrentTools = 1
	config.ToolTimeout = 100 * time.Millisecond
	executor := NewToolExecutorWithConfig(registry, config)

	results := make(chan types.ToolResult, 2)
	go func() {
		results <- executor.Execute(context.Background(), types.ToolCall{ID: "first", Name: "slow_tool", Arguments: map[string]any{}})
	}()
	<-firstStarted
	go func() {
		results <- executor.Execute(context.Background(), types.ToolCall{ID: "second", Name: "slow_tool", Arguments: map[string]any{}})
	}()

	for range 2 {
		result := <-results
		if result.Error != "" || result.Result != "done" {
			t.Fatalf("queued tool result = %#v, want successful 75ms execution", result)
		}
	}
}
