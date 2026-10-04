package openai

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/internal/testutil"
	"github.com/garyblankenship/wormhole/v3/types"
)

// C4-02: interleaved fragments are observable before the terminal is available.
func TestRemediationC402LiveInterleavedToolFragments(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := make(chan types.StreamChunk)
	output := (&Provider{}).accumulatingStream(ctx, input)
	receive := func() types.StreamChunk {
		t.Helper()
		select {
		case chunk, ok := <-output:
			if !ok {
				t.Fatal("stream closed early")
			}
			return chunk
		case <-time.After(time.Second):
			t.Fatal("live fragment withheld until completion")
			return types.StreamChunk{}
		}
	}
	rawA, _ := json.Marshal(map[string]int{"a": 1})
	rawB, _ := json.Marshal(map[string]int{"b": 2})
	fragment := func(index int, id, name, raw string) types.ToolCall {
		return types.ToolCall{Index: index, ID: id, Name: name, Type: "function", Function: &types.ToolCallFunction{Name: name, Arguments: raw}}
	}
	first := []types.ToolCall{fragment(3, "call_a", "first", string(rawA[:len(rawA)-2])), fragment(7, "call_b", "second", string(rawB[:len(rawB)-2]))}
	input <- types.StreamChunk{Delta: &types.ChunkDelta{ToolCalls: first}, ToolCalls: first, ToolCall: &first[0]}
	chunk := receive()
	all := []types.StreamChunk{chunk}
	if chunk.Delta == nil || len(chunk.Delta.ToolCalls) != 2 || chunk.Delta.ToolCalls[0].Index != 3 || chunk.Delta.ToolCalls[1].Index != 7 || len(chunk.ToolCalls) != 0 || chunk.ToolCall != nil {
		t.Fatalf("first chunk=%#v", chunk)
	}
	for _, call := range []types.ToolCall{fragment(7, "", "", string(rawB[len(rawB)-2:])), fragment(3, "", "", string(rawA[len(rawA)-2:]))} {
		input <- types.StreamChunk{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{call}}}
		chunk = receive()
		all = append(all, chunk)
		if chunk.Delta == nil || len(chunk.Delta.ToolCalls) != 1 || chunk.Delta.ToolCalls[0].Index != call.Index || chunk.Delta.ToolCalls[0].Function.Arguments != call.Function.Arguments || len(chunk.ToolCalls) != 0 {
			t.Fatalf("continuation=%#v", chunk)
		}
	}
	reason := types.FinishReasonToolCalls
	input <- types.StreamChunk{FinishReason: &reason}
	terminal := receive()
	all = append(all, terminal)
	if terminal.Delta != nil && len(terminal.Delta.ToolCalls) > 0 {
		t.Fatal("terminal aggregate replayed as delta")
	}
	if len(terminal.ToolCalls) != 2 {
		t.Fatalf("terminal=%#v", terminal)
	}
	if terminal.ToolCalls[0].Index != 3 || terminal.ToolCalls[0].ID != "call_a" || terminal.ToolCalls[0].Arguments["a"] != float64(1) || terminal.ToolCalls[1].Index != 7 || terminal.ToolCalls[1].Arguments["b"] != float64(2) {
		t.Fatalf("calls=%#v", terminal.ToolCalls)
	}
	merged := testutil.MergeTextChunks(all)
	if len(merged.ToolCalls) != 2 {
		t.Fatalf("merged tools=%#v", merged.ToolCalls)
	}
	close(input)
	if _, ok := <-output; ok {
		t.Fatal("unexpected extra chunk")
	}
}

func TestRemediationC402AccumulatorCancelsBlockedReceive(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	output := (&Provider{}).accumulatingStream(ctx, make(chan types.StreamChunk))
	cancel()
	select {
	case _, ok := <-output:
		if ok {
			t.Fatal("unexpected chunk")
		}
	case <-time.After(time.Second):
		t.Fatal("accumulator blocked on upstream receive")
	}
}
