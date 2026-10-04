package providers

import (
	"reflect"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

// C4-03: results consume only an earlier unmatched occurrence, including ID reuse.
func TestRemediationC403OrderedToolHistory(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name                     string
		messages                 []types.Message
		calls, results, warnings int
	}{
		{"result before call", []types.Message{types.NewToolResultMessage("id", "early"), &types.AssistantMessage{Content: "retained", ToolCalls: []types.ToolCall{{ID: "id", Name: "tool"}}}}, 0, 0, 2},
		{"duplicate result", []types.Message{&types.AssistantMessage{ToolCalls: []types.ToolCall{{ID: "id", Name: "tool"}}}, types.NewToolResultMessage("id", "one"), types.NewToolResultMessage("id", "two")}, 1, 1, 1},
		{"later reuse unmatched", []types.Message{&types.AssistantMessage{ToolCalls: []types.ToolCall{{ID: "id", Name: "tool"}}}, types.NewToolResultMessage("id", "one"), &types.AssistantMessage{ToolCalls: []types.ToolCall{{ID: "id", Name: "tool"}}}}, 1, 1, 1},
		{"later reuse matched", []types.Message{&types.AssistantMessage{ToolCalls: []types.ToolCall{{ID: "id", Name: "tool"}}}, types.NewToolResultMessage("id", "one"), &types.AssistantMessage{ToolCalls: []types.ToolCall{{ID: "id", Name: "tool"}}}, types.NewToolResultMessage("id", "two")}, 2, 2, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			before := types.CloneMessages(test.messages)
			repaired, warnings, err := PrepareMessages(test.messages)
			if err != nil {
				t.Fatal(err)
			}
			calls, results := 0, 0
			for _, message := range repaired {
				switch m := message.(type) {
				case *types.AssistantMessage:
					calls += len(m.ToolCalls)
				case *types.ToolResultMessage:
					results++
				}
			}
			if calls != test.calls || results != test.results || len(warnings) != test.warnings {
				t.Fatalf("calls=%d results=%d warnings=%v", calls, results, warnings)
			}
			validation, err := ValidateMessageSequence(test.messages)
			if err != nil || !reflect.DeepEqual(validation, warnings) {
				t.Fatalf("validation=%v err=%v", validation, err)
			}
			if !reflect.DeepEqual(before, test.messages) {
				t.Fatal("caller history mutated")
			}
		})
	}
}

func TestRemediationC403NormalizedDuplicatesAndThinking(t *testing.T) {
	t.Parallel()
	duplicate := []types.Message{&types.AssistantMessage{ToolCalls: []types.ToolCall{{ID: "a:b", Name: "one"}, {ID: "a;b", Name: "two"}}}}
	if _, err := ValidateMessageSequence(duplicate); err == nil {
		t.Fatal("normalized duplicate accepted")
	}
	thinking := &types.Thinking{Content: "reason", Signature: "signed", Provider: "anthropic"}
	input := &types.AssistantMessage{Content: "text", Thinking: thinking, ToolCalls: []types.ToolCall{{ID: "a:b", Name: "one"}}}
	repaired, _, err := PrepareMessages([]types.Message{input, types.NewToolResultMessage("a:b", "ok")})
	if err != nil {
		t.Fatal(err)
	}
	output := repaired[0].(*types.AssistantMessage)
	if output.Content != "text" || !reflect.DeepEqual(output.Thinking, thinking) || output.ToolCalls[0].ID != "a_b" || input.ToolCalls[0].ID != "a:b" {
		t.Fatalf("output=%#v input=%#v", output, input)
	}
}
