package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

// C6-03: buildContent must not emit an empty text block when c == "" and other
// content blocks (tool_use, media, thinking) are present.
func TestRemediationC603_NoEmptyTextBlockWithOtherBlocks(t *testing.T) {
	t.Parallel()
	p := &Provider{}

	t.Run("assistant with empty text and tool_use", func(t *testing.T) {
		t.Parallel()
		parts := p.buildContent(&types.AssistantMessage{
			Content: "",
			ToolCalls: []types.ToolCall{
				{ID: "call-1", Name: "lookup", Arguments: map[string]any{"q": "x"}},
			},
		})
		require.Len(t, parts, 1)
		assert.Equal(t, "tool_use", parts[0]["type"])
	})

	t.Run("assistant with empty text, thinking, and tool_use", func(t *testing.T) {
		t.Parallel()
		parts := p.buildContent(&types.AssistantMessage{
			Content:  "",
			Thinking: &types.Thinking{Content: "hmm", Signature: "sig-1", Provider: "anthropic"},
			ToolCalls: []types.ToolCall{
				{ID: "call-1", Name: "lookup", Arguments: map[string]any{}},
			},
		})
		require.Len(t, parts, 2)
		assert.Equal(t, "thinking", parts[0]["type"])
		assert.Equal(t, "tool_use", parts[1]["type"])
	})

	t.Run("user with empty text and image media", func(t *testing.T) {
		t.Parallel()
		parts := p.buildContent(&types.UserMessage{
			Content: "",
			Media: []types.Media{
				&types.ImageMedia{MimeType: "image/png", Base64Data: "aW1hZ2U="},
			},
		})
		require.Len(t, parts, 1)
		assert.Equal(t, "image", parts[0]["type"])
	})
}

// C6-06: stampProvider must clone ResponseMetadata across chunks so callers
// mutating metadata on one chunk cannot corrupt another.
func TestRemediationC606_StampProviderClonesMetadata(t *testing.T) {
	t.Parallel()
	p := New(types.ProviderConfig{APIKey: "test-key"})

	in := make(chan types.StreamChunk, 2)
	in <- types.StreamChunk{Delta: &types.ChunkDelta{Content: "hi"}}
	reason := types.FinishReasonStop
	in <- types.StreamChunk{FinishReason: &reason}
	close(in)

	meta := &types.ResponseMetadata{
		RequestID: "req-1",
		Headers:   map[string][]string{"X-Test": {"orig"}},
	}

	out := p.stampProvider(context.Background(), in, meta, nil)
	var chunks []types.StreamChunk
	for ch := range out {
		chunks = append(chunks, ch)
	}
	require.Len(t, chunks, 2)
	require.NotNil(t, chunks[0].ResponseMetadata)
	require.NotNil(t, chunks[1].ResponseMetadata)
	assert.NotSame(t, chunks[0].ResponseMetadata, chunks[1].ResponseMetadata)
	chunks[0].ResponseMetadata.Headers["X-Test"][0] = "mutated"
	assert.Equal(t, []string{"orig"}, chunks[1].ResponseMetadata.Headers["X-Test"])
}

// C6-10: ToolMessage with empty Content and non-empty Error uses Error as content;
// transformTools defaults nil schema to {"type":"object","properties":{}}.
func TestRemediationC610_ToolErrorFallbackAndDefaultSchema(t *testing.T) {
	t.Parallel()
	p := &Provider{}

	t.Run("tool error fallback when content is empty", func(t *testing.T) {
		t.Parallel()
		parts := p.buildContent(&types.ToolMessage{
			ToolCallID: "call-err",
			Content:    "",
			Error:      "permission denied",
		})
		require.Len(t, parts, 1)
		assert.Equal(t, "permission denied", parts[0]["content"])
		assert.Equal(t, true, parts[0]["is_error"])
	})

	t.Run("nil tool schema defaults to empty object schema", func(t *testing.T) {
		t.Parallel()
		tools, err := p.transformTools([]types.Tool{
			{Name: "ping", Description: "Ping the server"},
		})
		require.NoError(t, err)
		require.Len(t, tools, 1)
		raw, ok := tools[0]["input_schema"].(json.RawMessage)
		require.True(t, ok)
		var schema map[string]any
		require.NoError(t, json.Unmarshal(raw, &schema))
		assert.Equal(t, "object", schema["type"])
		assert.Equal(t, map[string]any{}, schema["properties"])
	})
}

// C6-13: parseStreamChunk returns StreamChunk{Error: *types.WormholeError} for
// SSE error events so provider error classification is preserved.
func TestRemediationC613_SSEErrorPreservesWormholeError(t *testing.T) {
	t.Parallel()
	p := New(types.ProviderConfig{APIKey: "test-key"})

	chunk, err := p.parseStreamChunk([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`))
	require.NoError(t, err)
	require.NotNil(t, chunk)
	require.Error(t, chunk.Error)

	var wErr *types.WormholeError
	require.True(t, errors.As(chunk.Error, &wErr))
	assert.Equal(t, types.ErrorCodeProvider, wErr.Code)
	assert.Equal(t, "anthropic", wErr.Provider)
}
