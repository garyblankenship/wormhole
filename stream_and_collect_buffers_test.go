package wormhole_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestStreamAndCollectRawToolsBufferedIsolation(t *testing.T) {
	t.Parallel()
	fragment := func(index int, id, name, raw string) types.TextChunk {
		return types.TextChunk{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{
			Index: index, ID: id, ThoughtSignature: "signature",
			Function: &types.ToolCallFunction{Name: name, Arguments: raw},
		}}}}
	}
	for _, terminal := range []bool{false, true} {
		name := "assembled"
		if terminal {
			name = "terminal replacement"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			chunks := []types.TextChunk{
				fragment(7, "first", "lookup", `{"value":["`),
				fragment(2, "second", "other", `{"value":"`),
				fragment(9, "third", "broken", `{"unfinished":`),
				fragment(2, "", "", `second"}`),
				fragment(7, "", "", `original"]}`),
			}
			if terminal {
				stop := types.FinishReasonToolCalls
				chunks = append(chunks, types.TextChunk{FinishReason: &stop, ToolCalls: []types.ToolCall{{
					ID: "replacement", Name: "final", Arguments: map[string]any{"value": []any{"original"}},
					Function: &types.ToolCallFunction{Name: "final", Arguments: `{"value":["original"]}`},
				}}}, fragment(7, "late", "ignored", `late bytes`))
			} else {
				stop := types.FinishReasonToolCalls
				chunks = append(chunks, types.TextChunk{FinishReason: &stop})
			}
			client := collectFixture(t, chunks)
			stream, result, err := client.Text().Model("test-model").Prompt("test").StreamAndCollect(context.Background())
			require.NoError(t, err)
			for chunk := range stream {
				if chunk.Delta != nil && len(chunk.Delta.ToolCalls) > 0 {
					chunk.Delta.ToolCalls[0].Function.Arguments = "consumer mutation"
				}
			}
			got, err := result()
			require.NoError(t, err)
			if terminal {
				require.Len(t, got.ToolCalls, 1)
				require.Equal(t, "replacement", got.ToolCalls[0].ID)
			} else {
				require.Len(t, got.ToolCalls, 3)
				require.Equal(t, []int{7, 2, 9}, []int{got.ToolCalls[0].Index, got.ToolCalls[1].Index, got.ToolCalls[2].Index})
				require.Equal(t, "signature", got.ToolCalls[0].ThoughtSignature)
				require.Equal(t, "second", got.ToolCalls[1].Arguments["value"])
				require.True(t, got.ToolCalls[2].ArgsInvalid)
				require.Equal(t, `{"unfinished":`, got.ToolCalls[2].Function.Arguments)
				got.ToolCalls[2].Function.Arguments = "mutated invalid raw bytes"
			}
			require.Equal(t, `{"value":["original"]}`, got.ToolCalls[0].Function.Arguments)
			got.ToolCalls[0].Arguments["value"].([]any)[0] = "changed"
			got.ToolCalls[0].Function.Arguments = "changed"
			again, err := result()
			require.NoError(t, err)
			require.Equal(t, "original", again.ToolCalls[0].Arguments["value"].([]any)[0])
			require.Equal(t, `{"value":["original"]}`, again.ToolCalls[0].Function.Arguments)
			if !terminal {
				require.Equal(t, `{"unfinished":`, again.ToolCalls[2].Function.Arguments)
			}
		})
	}
}
