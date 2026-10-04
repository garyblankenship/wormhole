package anthropic

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/internal/testutil"
	"github.com/garyblankenship/wormhole/v3/types"
)

// C4-02: interleaved fragments are consumed, never exposed; completes carry
// the provider wire indices and are emitted exactly once.
func TestRemediationC402InterleavedToolFragments(t *testing.T) {
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
			t.Fatal("chunk withheld until completion")
			return types.StreamChunk{}
		}
	}
	emptyOfFragments := func(chunk types.StreamChunk, stage string) {
		t.Helper()
		if chunk.ToolCall != nil {
			t.Fatalf("%s: singular fragment escaped: %#v", stage, chunk.ToolCall)
		}
		if len(chunk.ToolCalls) != 0 {
			t.Fatalf("%s: fragments escaped as calls: %#v", stage, chunk.ToolCalls)
		}
		if chunk.Delta != nil && len(chunk.Delta.ToolCalls) != 0 {
			t.Fatalf("%s: fragments escaped as deltas: %#v", stage, chunk.Delta.ToolCalls)
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
	emptyOfFragments(chunk, "first")
	for _, call := range []types.ToolCall{fragment(7, "", "", string(rawB[len(rawB)-2:])), fragment(3, "", "", string(rawA[len(rawA)-2:]))} {
		input <- types.StreamChunk{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{call}}}
		chunk = receive()
		all = append(all, chunk)
		emptyOfFragments(chunk, "continuation")
	}
	reason := types.FinishReasonToolCalls
	input <- types.StreamChunk{FinishReason: &reason}
	terminal := receive()
	all = append(all, terminal)
	if len(terminal.ToolCalls) != 2 {
		t.Fatalf("terminal=%#v", terminal)
	}
	if terminal.ToolCalls[0].Index != 3 || terminal.ToolCalls[0].ID != "call_a" || terminal.ToolCalls[0].Arguments["a"] != float64(1) || terminal.ToolCalls[1].Index != 7 || terminal.ToolCalls[1].ID != "call_b" || terminal.ToolCalls[1].Arguments["b"] != float64(2) {
		t.Fatalf("calls=%#v", terminal.ToolCalls)
	}
	input <- types.StreamChunk{FinishReason: &reason}
	if again := receive(); len(again.ToolCalls) != 0 {
		t.Fatalf("repeated finish re-emitted calls: %#v", again.ToolCalls)
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

// C4-02: the parser preserves nonzero content-block indices before accumulation.
func TestRemediationC402AnthropicWireBlockIndex(t *testing.T) {
	t.Parallel()
	provider := &Provider{}
	start := contentBlockStartEvent{Type: "content_block_start", Index: 9}
	start.ContentBlock.Type = "tool_use"
	start.ContentBlock.ID = "tool_id"
	start.ContentBlock.Name = "tool"
	raw, _ := json.Marshal(start)
	chunk, err := provider.parseStreamChunk(raw)
	if err != nil || chunk.Delta == nil || chunk.Delta.ToolCalls[0].Index != 9 {
		t.Fatalf("chunk=%#v err=%v", chunk, err)
	}
	delta := contentBlockDeltaEvent{Type: "content_block_delta", Index: 9}
	delta.Delta.Type = "input_json_delta"
	args, _ := json.Marshal(map[string]int{"value": 1})
	delta.Delta.PartialJSON = string(args)
	raw, _ = json.Marshal(delta)
	chunk, err = provider.parseStreamChunk(raw)
	if err != nil || chunk.Delta == nil || chunk.Delta.ToolCalls[0].Index != 9 || chunk.Delta.ToolCalls[0].Function.Arguments != string(args) {
		t.Fatalf("chunk=%#v err=%v", chunk, err)
	}
}
