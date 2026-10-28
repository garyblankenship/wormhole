package server

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func chatWireDeltas(t *testing.T, body string) []map[string]json.RawMessage {
	t.Helper()
	var deltas []map[string]json.RawMessage
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		var response struct {
			Choices []struct {
				Delta map[string]json.RawMessage `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Choices) != 1 {
			t.Fatalf("choices = %d, want 1", len(response.Choices))
		}
		if response.Choices[0].Delta == nil {
			t.Fatal("delta must be a present, non-null object")
		}
		deltas = append(deltas, response.Choices[0].Delta)
	}
	return deltas
}

func TestChatDeltaWireFields(t *testing.T) {
	t.Parallel()
	finish := types.FinishReasonToolCalls
	chunks := []types.TextChunk{
		{},
		{Text: "hello"},
		{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{Index: 3, ID: "upstream", Name: "lookup", Function: &types.ToolCallFunction{Arguments: "{"}}}}},
		{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{Index: 3, Function: &types.ToolCallFunction{Arguments: "}"}}}}},
		{FinishReason: &finish},
	}
	state := newChatStreamState("fixture", nil)
	recorder := httptest.NewRecorder()
	for _, chunk := range chunks {
		if !state.frame(&proxy{}, recorder, recorder, chunk) {
			t.Fatal("frame failed")
		}
	}
	deltas := chatWireDeltas(t, recorder.Body.String())
	if len(deltas) != len(chunks) {
		t.Fatalf("deltas = %d, want %d", len(deltas), len(chunks))
	}
	var role string
	if err := json.Unmarshal(deltas[0]["role"], &role); err != nil || role != "assistant" {
		t.Fatalf("opening role = %q, error = %v", role, err)
	}
	if len(deltas[0]) != 1 {
		t.Fatalf("opening delta fields = %v, want only role", deltas[0])
	}
	for i, delta := range deltas[1:] {
		if _, exists := delta["role"]; exists {
			t.Fatalf("delta %d repeated role", i+1)
		}
	}
	if len(deltas[1]) != 1 || string(deltas[1]["content"]) != `"hello"` {
		t.Fatalf("text delta = %v", deltas[1])
	}
	for _, delta := range deltas[2:4] {
		if len(delta) != 1 {
			t.Fatalf("tool delta fields = %v, want only tool_calls", delta)
		}
		var calls []ChatToolCall
		if err := json.Unmarshal(delta["tool_calls"], &calls); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 1 || calls[0].Index == nil || *calls[0].Index != 3 {
			t.Fatalf("tool indices = %+v, want 3", calls)
		}
	}
	if len(deltas[4]) != 0 {
		t.Fatalf("terminal delta = %v, want {}", deltas[4])
	}
}

func TestChatDeltaTerminalPreservesColocatedText(t *testing.T) {
	t.Parallel()
	finish := types.FinishReasonStop
	state := newChatStreamState("fixture", nil)
	recorder := httptest.NewRecorder()
	if !state.frame(&proxy{}, recorder, recorder, types.TextChunk{Text: "last", FinishReason: &finish}) {
		t.Fatal("frame failed")
	}
	deltas := chatWireDeltas(t, recorder.Body.String())
	if len(deltas) != 2 || string(deltas[0]["content"]) != `"last"` || len(deltas[1]) != 0 {
		t.Fatalf("deltas = %v, want text followed by {}", deltas)
	}
}

func TestChatDeltaExtensionTerminalIsEmpty(t *testing.T) {
	t.Parallel()
	state := newChatStreamState("fixture", &WormholeRequest{Include: []string{"usage"}})
	recorder := httptest.NewRecorder()
	state.finish(&proxy{}, recorder, recorder)
	deltas := chatWireDeltas(t, recorder.Body.String())
	if len(deltas) != 1 || len(deltas[0]) != 0 {
		t.Fatalf("extension terminal deltas = %v, want [{}]", deltas)
	}
}

func TestChatMessageRetainsRequiredWireFields(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(ChatMessage{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fields, map[string]string{"role": "", "content": ""}) {
		t.Fatalf("full message fields = %v", fields)
	}
}

func TestFromWormholeToolCallsMissingIDsAvoidCollisions(t *testing.T) {
	t.Parallel()
	input := []types.ToolCall{{Name: "first"}, {ID: "call_1", Name: "supplied"}, {Name: "second"}, {ID: "upstream", Name: "other"}}
	want := []string{"call_2", "call_1", "call_3", "upstream"}
	for range 2 {
		got := fromWormholeToolCalls(input)
		for i, call := range got {
			if call.ID != want[i] {
				t.Fatalf("call %d ID = %q, want %q", i, call.ID, want[i])
			}
		}
	}
	if input[0].ID != "" || input[2].ID != "" {
		t.Fatal("conversion mutated input IDs")
	}
}
