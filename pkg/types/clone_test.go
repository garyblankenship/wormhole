package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeToolCallCanonicalizesRepresentations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call ToolCall
		want string
		args map[string]any
	}{
		{
			name: "top level wins name",
			call: ToolCall{Name: "top", Arguments: map[string]any{"count": 2}, Function: &ToolCallFunction{Name: "nested", Arguments: `{"count":2}`}},
			want: "top",
			args: map[string]any{"count": 2},
		},
		{
			name: "nested fills missing",
			call: ToolCall{Function: &ToolCallFunction{Name: "nested", Arguments: `{"city":"Paris"}`}},
			want: "nested",
			args: map[string]any{"city": "Paris"},
		},
		{
			name: "no arguments",
			call: ToolCall{Name: "ping"},
			want: "ping",
			args: map[string]any{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, err := NormalizeToolCall(test.call)
			require.NoError(t, err)
			assert.Equal(t, test.want, normalized.Name)
			assert.Equal(t, test.want, normalized.Function.Name)
			assert.Equal(t, test.args, normalized.Arguments)
			assert.JSONEq(t, mustJSON(t, test.args), normalized.Function.Arguments)
		})
	}
}

func TestNormalizeToolCallRejectsMalformedAndConflictingArguments(t *testing.T) {
	t.Parallel()

	tests := []ToolCall{
		{Name: "bad", Function: &ToolCallFunction{Arguments: `{"broken"`}},
		{Name: "array", Function: &ToolCallFunction{Arguments: `[]`}},
		{Name: "conflict", Arguments: map[string]any{"a": 1}, Function: &ToolCallFunction{Arguments: `{"a":2}`}},
		{Name: "invalid", ArgsInvalid: true, ArgsParseError: "truncated"},
	}
	for _, call := range tests {
		_, err := NormalizeToolCall(call)
		require.Error(t, err)
	}
}

func TestCloneMessageDetachesNestedState(t *testing.T) {
	t.Parallel()

	original := &UserMessage{Media: []Media{&ImageMedia{Data: []byte("image")}}}
	clone := CloneMessage(original).(*UserMessage)
	clone.Media[0].(*ImageMedia).Data[0] = 'X'
	assert.Equal(t, []byte("image"), original.Media[0].(*ImageMedia).Data)

	assistant := &AssistantMessage{ToolCalls: []ToolCall{{Arguments: map[string]any{"nested": map[string]any{"value": true}}}}}
	assistantClone := CloneMessage(assistant).(*AssistantMessage)
	assistantClone.ToolCalls[0].Arguments["nested"].(map[string]any)["value"] = false
	assert.Equal(t, true, assistant.ToolCalls[0].Arguments["nested"].(map[string]any)["value"])
}

func TestCloneValueDetachesTypedMapsAndSlices(t *testing.T) {
	t.Parallel()

	original := map[string]any{
		"labels": map[string]string{"env": "prod"},
		"ports":  []int{80, 443},
		"routes": map[string][]string{"api": {"primary", "fallback"}},
	}
	clone := CloneValue(original).(map[string]any)
	clone["labels"].(map[string]string)["env"] = "test"
	clone["ports"].([]int)[0] = 8080
	clone["routes"].(map[string][]string)["api"][0] = "changed"

	assert.Equal(t, "prod", original["labels"].(map[string]string)["env"])
	assert.Equal(t, 80, original["ports"].([]int)[0])
	assert.Equal(t, "primary", original["routes"].(map[string][]string)["api"][0])
}

func TestCloneValuePreservesCyclesWithoutSharing(t *testing.T) {
	t.Parallel()

	original := map[string]any{"value": "original"}
	original["self"] = original
	clone := CloneValue(original).(map[string]any)
	clone["self"].(map[string]any)["value"] = "changed"

	assert.Equal(t, "changed", clone["value"])
	assert.Equal(t, "original", original["value"])
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
