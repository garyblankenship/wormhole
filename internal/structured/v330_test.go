package structured

import (
	"context"
	"errors"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestV330StructuredAdaptation(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}, "required": []string{"value"}}
	app := *types.NewTool("lookup", "lookup", map[string]any{"type": "object"})
	for _, test := range []struct {
		name     string
		choice   *types.ToolChoice
		count    int
		selected string
	}{
		{"auto", nil, 2, ""}, {"none", &types.ToolChoice{Type: types.ToolChoiceTypeNone}, 1, ResultToolName}, {"specific", &types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: "lookup"}, 2, "lookup"}, {"required", &types.ToolChoice{Type: types.ToolChoiceTypeAny}, 1, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request, err := Prepare(types.StructuredRequest{Schema: schema, Tools: []types.Tool{app}, ToolChoice: test.choice})
			if err != nil {
				t.Fatal(err)
			}
			if len(request.Tools) != test.count {
				t.Fatalf("tool count %d", len(request.Tools))
			}
			if test.selected != "" && request.ToolChoice.ToolName != test.selected {
				t.Fatal("choice changed")
			}
		})
	}
	_, err := Prepare(types.StructuredRequest{Schema: schema, Tools: []types.Tool{*types.NewTool(ResultToolName, "reserved", nil)}})
	if err == nil {
		t.Fatal("reserved name accepted")
	}
	_, err = FinishTools(schema, &types.TextResponse{ToolCalls: []types.ToolCall{{Name: ResultToolName, Arguments: map[string]any{"value": 1}}}})
	if err == nil {
		t.Fatal("invalid schema accepted")
	}
	original := &types.TextResponse{ToolCalls: []types.ToolCall{{Name: "lookup", Arguments: map[string]any{}}, {Name: ResultToolName, Arguments: map[string]any{"value": "ok"}}}}
	_, err = FinishTools(schema, original)
	var invalid *ResponseError
	if !errors.As(err, &invalid) || invalid.Response != original {
		t.Fatal("mixed calls did not retain response")
	}
}

func TestV330StructuredStreamingFallback(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "required": []string{"value"}, "properties": map[string]any{"value": map[string]any{"type": "string"}}}
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "valid", false: "invalid"}[valid], func(t *testing.T) {
			t.Parallel()
			input := make(chan types.TextChunk, 3)
			arguments := `{"value":"ok"}`
			if !valid {
				arguments = `{"value":1}`
			}
			input <- types.TextChunk{ToolCalls: []types.ToolCall{{Index: 0, ID: "final", Name: ResultToolName, Function: &types.ToolCallFunction{Name: ResultToolName, Arguments: arguments[:5]}}}}
			input <- types.TextChunk{ToolCalls: []types.ToolCall{{Index: 0, Function: &types.ToolCallFunction{Arguments: arguments[5:]}}}}
			finish := types.FinishReasonToolCalls
			input <- types.TextChunk{FinishReason: &finish}
			close(input)
			var chunks []types.TextChunk
			for chunk := range StreamTools(context.Background(), schema, input) {
				chunks = append(chunks, chunk)
			}
			if len(chunks) != 1 {
				t.Fatalf("result arguments escaped buffering: %#v", chunks)
			}
			if valid {
				if chunks[0].Text != arguments || chunks[0].FinishReason == nil || chunks[0].Error != nil {
					t.Fatalf("invalid final %#v", chunks[0])
				}
			} else if chunks[0].Error == nil || chunks[0].FinishReason != nil {
				t.Fatal("invalid JSON completed successfully")
			}
		})
	}
}
