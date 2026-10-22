package structured

import (
	"context"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestV330SchemaReferenceBounds(t *testing.T) {
	t.Parallel()
	for name, schema := range map[string]map[string]any{
		"self":       {"$ref": "#"},
		"indirect":   {"$ref": "#/$defs/a", "$defs": map[string]any{"a": map[string]any{"$ref": "#/$defs/b"}, "b": map[string]any{"$ref": "#/$defs/a"}}},
		"applicator": {"anyOf": []any{map[string]any{"$ref": "#"}, map[string]any{"type": "null"}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := Validate(schema, nil); err == nil || !strings.Contains(err.Error(), "cycle") {
				t.Fatalf("expected bounded cycle rejection, got %v", err)
			}
			if _, err := PrepareTools(types.TextRequest{}, schema); err == nil {
				t.Fatal("cyclic schema reached request preparation")
			}
		})
	}
	recursive := map[string]any{"type": "object", "properties": map[string]any{"next": map[string]any{"$ref": "#"}, "value": map[string]any{"type": "integer"}}}
	if err := Validate(recursive, map[string]any{"next": map[string]any{"value": 2}}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(recursive, map[string]any{"next": map[string]any{"value": "invalid"}}); err == nil {
		t.Fatal("recursive child was not validated")
	}
	var deep any = map[string]any{}
	for range maxSchemaDepth + 1 {
		deep = map[string]any{"next": deep}
	}
	if err := Validate(recursive, deep); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("deep validation not bounded: %v", err)
	}
	// A wide uniqueItems check must also stop, even without deep recursion.
	values := make([]any, 600)
	for i := range values {
		values[i] = i
	}
	if err := Validate(map[string]any{"uniqueItems": true}, values); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("wide validation not bounded: %v", err)
	}
	// Complexity failure cannot be hidden by anyOf or not matching another branch.
	guarded := map[string]any{"anyOf": []any{map[string]any{"uniqueItems": true}, map[string]any{}}}
	if err := Validate(guarded, values); err == nil {
		t.Fatal("anyOf swallowed work limit")
	}
}

func TestV330SchemaAssertionsFailClosed(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name           string
		divisor, value any
		valid          bool
	}{
		{"integer good", 2, 4, true}, {"integer bad", 2, 3, false},
		{"decimal good", 0.1, 0.3, true}, {"decimal bad", 0.2, 0.3, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			schema := map[string]any{"type": "number", "multipleOf": test.divisor}
			err := Validate(schema, test.value)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, err=%v", test.valid, err)
			}
			// Test final-result enforcement at the real object boundary too.
			object := map[string]any{"type": "object", "properties": map[string]any{"n": schema}}
			_, err = FinishTools(object, &types.TextResponse{ToolCalls: []types.ToolCall{{Name: ResultToolName, Arguments: map[string]any{"n": test.value}}}})
			if (err == nil) != test.valid {
				t.Fatalf("final-result valid=%v, err=%v", test.valid, err)
			}
		})
	}
	for _, keyword := range []string{"contains", "patternProperties", "dependentRequired", "if", "format", "unevaluatedProperties"} {
		t.Run(keyword, func(t *testing.T) {
			t.Parallel()
			schema := map[string]any{"type": "object", "properties": map[string]any{"nested": map[string]any{keyword: true}}}
			if _, err := PrepareTools(types.TextRequest{}, schema); err == nil || !strings.Contains(err.Error(), keyword) {
				t.Fatalf("unsupported assertion accepted: %v", err)
			}
			for _, mode := range []types.StructuredMode{types.StructuredModeJSON, types.StructuredModeStrict, types.StructuredModeTools} {
				if _, err := Prepare(types.StructuredRequest{Schema: schema, Mode: mode}); err == nil {
					t.Fatalf("mode %s accepted unsupported assertion", mode)
				}
			}
		})
	}
	// $ref assertions and their siblings both apply.
	schema := map[string]any{"$ref": "#/$defs/n", "$defs": map[string]any{"n": map[string]any{"type": "number"}}, "minimum": 2}
	if err := Validate(schema, 1); err == nil {
		t.Fatal("ignored $ref sibling")
	}
}

func TestV330StructuredPrefixToolStreaming(t *testing.T) {
	t.Parallel()
	for _, nameParts := range [][]string{{"__wormhole"}, {"__worm", "hole"}, {"__wormhole", "_lookup"}, {"lookup"}} {
		t.Run(strings.Join(nameParts, "+"), func(t *testing.T) {
			t.Parallel()
			input := make(chan types.TextChunk, len(nameParts)+2)
			for i, name := range nameParts {
				input <- types.TextChunk{ToolCalls: []types.ToolCall{{Index: 0, ID: "app", Name: name, Function: &types.ToolCallFunction{Name: name}}}}
				if i == 0 {
					input <- types.TextChunk{ToolCall: &types.ToolCall{Index: 0, Function: &types.ToolCallFunction{Arguments: "{}"}, Arguments: map[string]any{"value": "kept"}}}
				}
			}
			finish := types.FinishReasonToolCalls
			input <- types.TextChunk{FinishReason: &finish}
			close(input)
			var got []types.ToolCall
			for chunk := range StreamTools(context.Background(), map[string]any{}, input) {
				if chunk.Error != nil {
					t.Fatal(chunk.Error)
				}
				got = append(got, chunk.ToolCalls...)
			}
			if len(got) != len(nameParts)+1 {
				t.Fatalf("lost application deltas: %#v", got)
			}
			var names, args string
			preserved := false
			for _, call := range got {
				names += call.Name
				if call.Function != nil {
					args += call.Function.Arguments
				}
				if call.Arguments["value"] == "kept" {
					preserved = true
				}
			}
			if names != strings.Join(nameParts, "") || args != "{}" || !preserved {
				t.Fatalf("changed application turn: %#v", got)
			}
		})
	}
}

func TestV330StructuredFragmentedSyntheticAndMixed(t *testing.T) {
	t.Parallel()
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "synthetic", true: "mixed"}[mixed], func(t *testing.T) {
			t.Parallel()
			input := make(chan types.TextChunk, 4)
			input <- types.TextChunk{ToolCalls: []types.ToolCall{{Index: 0, ID: "result", Name: "__wormhole", Function: &types.ToolCallFunction{Arguments: "{"}}}}
			input <- types.TextChunk{ToolCalls: []types.ToolCall{{Index: 0, Name: "_structured_output", Function: &types.ToolCallFunction{Arguments: "}"}}}}
			if mixed {
				input <- types.TextChunk{ToolCalls: []types.ToolCall{{Index: 1, ID: "app", Name: "__wormhole", Arguments: map[string]any{}}}}
			}
			finish := types.FinishReasonToolCalls
			input <- types.TextChunk{FinishReason: &finish}
			close(input)
			count := 0
			for chunk := range StreamTools(context.Background(), map[string]any{"type": "object"}, input) {
				count++
				if mixed {
					if chunk.Error == nil || chunk.FinishReason != nil {
						t.Fatalf("mixed turn accepted: %#v", chunk)
					}
				} else if chunk.Error != nil || chunk.Text != "{}" || len(chunk.ToolCalls) != 0 {
					t.Fatalf("synthetic delta leaked: %#v", chunk)
				}
			}
			if count != 1 {
				t.Fatalf("unexpected buffered chunks: %d", count)
			}
		})
	}
}
