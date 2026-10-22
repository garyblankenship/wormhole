package types

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloneTextResponse(t *testing.T) {
	t.Parallel()
	source := &TextResponse{Thinking: &Thinking{Signature: "signature"}, Usage: &Usage{PromptTokens: 4},
		Metadata:  map[string]any{"nested": []any{map[string]any{"value": "original"}}},
		ToolCalls: []ToolCall{{Arguments: map[string]any{"nested": []any{"original"}}, Function: &ToolCallFunction{Name: "original"}}}}
	copy := CloneTextResponse(source)
	copy.Thinking.Signature = "changed"
	copy.Usage.PromptTokens = 99
	copy.Metadata["nested"].([]any)[0].(map[string]any)["value"] = "changed"
	copy.ToolCalls[0].Arguments["nested"].([]any)[0] = "changed"
	copy.ToolCalls[0].Function.Name = "changed"
	require.Equal(t, "signature", source.Thinking.Signature)
	require.Equal(t, 4, source.Usage.PromptTokens)
	require.Equal(t, "original", source.Metadata["nested"].([]any)[0].(map[string]any)["value"])
	require.Equal(t, "original", source.ToolCalls[0].Arguments["nested"].([]any)[0])
	require.Equal(t, "original", source.ToolCalls[0].Function.Name)
	require.Nil(t, CloneTextResponse(nil))
}

func TestCloneTextChunk(t *testing.T) {
	t.Parallel()
	reason := FinishReasonStop
	failure := errors.New("fixture")
	source := TextChunk{FinishReason: &reason, Error: failure, Usage: &Usage{PromptTokens: 4, CacheWriteTokensByTTL: map[string]int{"5m": 4}},
		Metadata:         map[string]any{"native": []string{"original"}},
		ResponseMetadata: &ResponseMetadata{Headers: map[string][]string{"Request-Id": {"original"}}},
		Sources:          []Source{{ID: "source", Raw: map[string]any{"title": "original"}}},
		Citations:        []Citation{{SourceID: "source", OutputRange: &TextRange{End: 1}}},
		Warnings:         []ResponseWarning{{Code: WarningMalformedCitation}},
		Thinking:         &Thinking{Content: "original"}, ToolCall: &ToolCall{Arguments: map[string]any{"value": "original"}},
		ToolCalls: []ToolCall{{Function: &ToolCallFunction{Name: "original"}}},
		Delta:     &ChunkDelta{Thinking: &Thinking{Content: "original"}, ToolCalls: []ToolCall{{Arguments: map[string]any{"value": "original"}}}}}
	copy := CloneTextChunk(source)
	*copy.FinishReason = FinishReasonLength
	copy.Usage.PromptTokens = 99
	copy.Thinking.Content = "changed"
	copy.ToolCall.Arguments["value"] = "changed"
	copy.ToolCalls[0].Function.Name = "changed"
	copy.Delta.Thinking.Content = "changed"
	copy.Delta.ToolCalls[0].Arguments["value"] = "changed"
	copy.Usage.CacheWriteTokensByTTL["5m"] = 99
	copy.Metadata["native"].([]string)[0] = "changed"
	copy.ResponseMetadata.Headers["Request-Id"][0] = "changed"
	copy.Sources[0].Raw["title"] = "changed"
	copy.Citations[0].OutputRange.End = 99
	copy.Warnings[0].Code = "changed"
	require.Equal(t, 4, source.Usage.CacheWriteTokensByTTL["5m"])
	require.Equal(t, "original", source.Metadata["native"].([]string)[0])
	require.Equal(t, "original", source.ResponseMetadata.Headers["Request-Id"][0])
	require.Equal(t, "original", source.Sources[0].Raw["title"])
	require.Equal(t, 1, source.Citations[0].OutputRange.End)
	require.Equal(t, WarningMalformedCitation, source.Warnings[0].Code)
	require.Equal(t, FinishReasonStop, reason)
	require.Equal(t, 4, source.Usage.PromptTokens)
	require.Equal(t, "original", source.Thinking.Content)
	require.Equal(t, "original", source.ToolCall.Arguments["value"])
	require.Equal(t, "original", source.ToolCalls[0].Function.Name)
	require.Equal(t, "original", source.Delta.Thinking.Content)
	require.Equal(t, "original", source.Delta.ToolCalls[0].Arguments["value"])
	require.ErrorIs(t, copy.Error, failure)
	require.Equal(t, TextChunk{}, CloneTextChunk(TextChunk{}))
}
