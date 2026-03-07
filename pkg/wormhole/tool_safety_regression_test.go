package wormhole

import (
	"context"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestToolExecutor_UnsupportedSafetySettingsFailFast(t *testing.T) {
	registry := NewToolRegistry()
	called := false

	registry.Register("test_tool", types.NewToolDefinition(types.Tool{
		Type:        "function",
		Name:        "test_tool",
		InputSchema: map[string]any{},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		called = true
		return map[string]any{"ok": true}, nil
	}))

	config := DefaultToolSafetyConfig()
	config.MaxMemoryMB = 128

	executor := NewToolExecutorWithConfig(registry, config)
	result := executor.Execute(context.Background(), types.ToolCall{
		ID:        "call_1",
		Name:      "test_tool",
		Arguments: map[string]any{},
	})

	assert.Contains(t, result.Error, "unsupported tool safety settings")
	assert.False(t, called)
}
