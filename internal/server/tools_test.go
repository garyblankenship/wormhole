package server

import (
	"encoding/json"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToWormholeTools(t *testing.T) {
	t.Parallel()

	got := toWormholeTools([]ChatTool{
		{
			Type: "function",
			Function: ChatToolFunction{
				Name:        "get_weather",
				Description: "d",
				Parameters:  map[string]any{"type": "object"},
			},
		},
	})

	require.Len(t, got, 1)
	assert.Equal(t, "function", got[0].Type)
	assert.Equal(t, "get_weather", got[0].Name)
	assert.Equal(t, "d", got[0].Description)
	assert.Equal(t, map[string]any{"type": "object"}, got[0].InputSchema)
}

func TestParseToolChoice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      json.RawMessage
		wantType types.ToolChoiceType
		wantName string
		nilWant  bool
	}{
		{name: "auto", raw: json.RawMessage(`"auto"`), wantType: types.ToolChoiceTypeAuto},
		{name: "none", raw: json.RawMessage(`"none"`), wantType: types.ToolChoiceTypeNone},
		{name: "required", raw: json.RawMessage(`"required"`), wantType: types.ToolChoiceTypeAny},
		{name: "specific", raw: json.RawMessage(`{"type":"function","function":{"name":"get_weather"}}`), wantType: types.ToolChoiceTypeSpecific, wantName: "get_weather"},
		{name: "empty raw", raw: nil, nilWant: true},
		{name: "unknown string", raw: json.RawMessage(`"weird"`), nilWant: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseToolChoice(tt.raw)
			if tt.nilWant {
				require.Nil(t, got)
				return
			}

			require.NotNil(t, got)
			assert.Equal(t, tt.wantType, got.Type)
			assert.Equal(t, tt.wantName, got.ToolName)
		})
	}
}

func TestToWormholeToolCalls(t *testing.T) {
	t.Parallel()

	t.Run("valid arguments", func(t *testing.T) {
		t.Parallel()

		got := toWormholeToolCalls([]ChatToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: ChatToolCallFunction{
					Name:      "get_weather",
					Arguments: `{"city":"NYC"}`,
				},
			},
		})

		require.Len(t, got, 1)
		assert.Equal(t, "call_1", got[0].ID)
		assert.Equal(t, "get_weather", got[0].Name)
		assert.Equal(t, `{"city":"NYC"}`, got[0].Function.Arguments)
		assert.Equal(t, map[string]any{"city": "NYC"}, got[0].Arguments)
	})

	t.Run("invalid arguments", func(t *testing.T) {
		t.Parallel()

		got := toWormholeToolCalls([]ChatToolCall{
			{
				ID:   "call_1",
				Type: "function",
				Function: ChatToolCallFunction{
					Name:      "get_weather",
					Arguments: `{bad`,
				},
			},
		})

		require.Len(t, got, 1)
		assert.Equal(t, "call_1", got[0].ID)
		assert.Equal(t, "get_weather", got[0].Name)
		assert.Empty(t, got[0].Arguments)
		assert.Equal(t, `{bad`, got[0].Function.Arguments)
	})
}

func TestFromWormholeToolCalls(t *testing.T) {
	t.Parallel()

	t.Run("from explicit function arguments", func(t *testing.T) {
		t.Parallel()

		got := fromWormholeToolCalls([]types.ToolCall{
			{
				ID:        "call_1",
				Name:      "get_weather",
				Type:      "function",
				Arguments: map[string]any{"ignored": "value"},
				Function: &types.ToolCallFunction{
					Name:      "get_weather",
					Arguments: `{"a":1}`,
				},
			},
		})

		require.Len(t, got, 1)
		assert.Equal(t, "call_1", got[0].ID)
		assert.Equal(t, "function", got[0].Type)
		assert.Equal(t, "get_weather", got[0].Function.Name)
		assert.Equal(t, `{"a":1}`, got[0].Function.Arguments)
	})

	t.Run("from argument map", func(t *testing.T) {
		t.Parallel()

		got := fromWormholeToolCalls([]types.ToolCall{
			{
				ID:        "call_2",
				Name:      "sum_numbers",
				Type:      "function",
				Arguments: map[string]any{"a": float64(1)},
			},
		})

		require.Len(t, got, 1)
		assert.Equal(t, "call_2", got[0].ID)
		assert.Equal(t, "function", got[0].Type)
		assert.Equal(t, "sum_numbers", got[0].Function.Name)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(got[0].Function.Arguments), &parsed))
		assert.Equal(t, map[string]any{"a": float64(1)}, parsed)
	})
}

func TestChunkToolFragments(t *testing.T) {
	t.Parallel()

	t.Run("delta preferred", func(t *testing.T) {
		t.Parallel()

		chunk := types.TextChunk{
			Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{
				ID:   "call_delta",
				Name: "weather",
				Function: &types.ToolCallFunction{
					Name:      "weather",
					Arguments: `{"city":"NYC"}`,
				},
			}}},
			ToolCalls: []types.ToolCall{{
				ID:   "call_chunk",
				Name: "ignored",
				Function: &types.ToolCallFunction{
					Name:      "ignored",
					Arguments: `{"city":"LA"}`,
				},
			}},
			ToolCall: &types.ToolCall{
				ID:   "call_single",
				Name: "ignored-single",
				Function: &types.ToolCallFunction{
					Name:      "ignored-single",
					Arguments: `{}`,
				},
			},
		}

		got := chunkToolFragments(chunk)
		require.Len(t, got, 1)
		assert.Equal(t, "call_delta", got[0].ID)
	})

	t.Run("chunk tool_calls used when delta absent", func(t *testing.T) {
		t.Parallel()

		chunk := types.TextChunk{
			ToolCalls: []types.ToolCall{{
				ID:       "call_chunk",
				Name:     "weather",
				Function: &types.ToolCallFunction{Name: "weather"},
			}},
		}
		got := chunkToolFragments(chunk)
		require.Len(t, got, 1)
		assert.Equal(t, "call_chunk", got[0].ID)
		assert.Equal(t, "weather", got[0].Name)
	})

	t.Run("single tool_call wrapped", func(t *testing.T) {
		t.Parallel()

		chunk := types.TextChunk{
			ToolCall: &types.ToolCall{
				ID:       "single",
				Name:     "weather",
				Function: &types.ToolCallFunction{Name: "weather"},
			},
		}

		got := chunkToolFragments(chunk)
		require.Len(t, got, 1)
		assert.Equal(t, "single", got[0].ID)
	})

	t.Run("empty chunk", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, chunkToolFragments(types.TextChunk{}))
	})
}

func TestStreamToolStateDelta(t *testing.T) {
	t.Parallel()

	t.Run("openai-style repeated ids", func(t *testing.T) {
		t.Parallel()

		state := newStreamToolState()

		d1 := state.delta(types.TextChunk{Delta: &types.ChunkDelta{
			ToolCalls: []types.ToolCall{{
				ID:       "call_1",
				Name:     "get_weather",
				Function: &types.ToolCallFunction{Name: "get_weather", Arguments: ""},
			}},
		}})
		require.Len(t, d1, 1)
		require.NotNil(t, d1[0].Index)
		assert.Equal(t, 0, *d1[0].Index)
		assert.Equal(t, "call_1", d1[0].ID)
		assert.Equal(t, "function", d1[0].Type)
		assert.Equal(t, "get_weather", d1[0].Function.Name)
		assert.Equal(t, "", d1[0].Function.Arguments)

		d2 := state.delta(types.TextChunk{Delta: &types.ChunkDelta{
			ToolCalls: []types.ToolCall{{
				ID:       "call_1",
				Function: &types.ToolCallFunction{Arguments: `{"ci`},
			}},
		}})
		require.Len(t, d2, 1)
		require.NotNil(t, d2[0].Index)
		assert.Equal(t, 0, *d2[0].Index)
		assert.Equal(t, "", d2[0].ID)
		assert.Empty(t, d2[0].Function.Name)
		assert.Equal(t, `{"ci`, d2[0].Function.Arguments)

		d3 := state.delta(types.TextChunk{Delta: &types.ChunkDelta{
			ToolCalls: []types.ToolCall{{
				ID:       "call_1",
				Function: &types.ToolCallFunction{Arguments: `ty":"NYC"}`},
			}},
		}})
		require.Len(t, d3, 1)
		require.NotNil(t, d3[0].Index)
		assert.Equal(t, 0, *d3[0].Index)
		assert.Equal(t, "", d3[0].ID)
		assert.Empty(t, d3[0].Function.Name)
		assert.Equal(t, `ty":"NYC"}`, d3[0].Function.Arguments)

		d4 := state.delta(types.TextChunk{Delta: &types.ChunkDelta{
			ToolCalls: []types.ToolCall{{
				ID:       "call_2",
				Name:     "forecast",
				Function: &types.ToolCallFunction{Name: "forecast", Arguments: ""},
			}},
		}})
		require.Len(t, d4, 1)
		require.NotNil(t, d4[0].Index)
		assert.Equal(t, 1, *d4[0].Index)
		assert.Equal(t, "call_2", d4[0].ID)
		assert.Equal(t, "forecast", d4[0].Function.Name)
	})

	t.Run("anthropic-style empty-id continuation", func(t *testing.T) {
		t.Parallel()

		state := newStreamToolState()

		d1 := state.delta(types.TextChunk{ToolCalls: []types.ToolCall{{
			ID:       "toolu_1",
			Name:     "get_weather",
			Function: &types.ToolCallFunction{Name: "get_weather"},
		}}})
		require.Len(t, d1, 1)
		require.NotNil(t, d1[0].Index)
		assert.Equal(t, 0, *d1[0].Index)
		assert.Equal(t, "toolu_1", d1[0].ID)
		assert.Equal(t, "get_weather", d1[0].Function.Name)

		d2 := state.delta(types.TextChunk{Delta: &types.ChunkDelta{
			ToolCalls: []types.ToolCall{{
				ID:       "",
				Function: &types.ToolCallFunction{Arguments: `{"x":1}`},
			}},
		}})
		require.Len(t, d2, 1)
		require.NotNil(t, d2[0].Index)
		assert.Equal(t, 0, *d2[0].Index)
		assert.Equal(t, "", d2[0].ID)
		assert.Empty(t, d2[0].Function.Name)
		assert.Equal(t, `{"x":1}`, d2[0].Function.Arguments)
	})
}
