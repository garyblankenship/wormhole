package wormhole_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/garyblankenship/wormhole/v3/wormholetest"
)

func collectClient(t *testing.T, provider types.Provider) *wormhole.Wormhole {
	t.Helper()
	client := wormhole.New(wormhole.WithCustomProvider("fixture", func(types.ProviderConfig) (types.Provider, error) { return provider, nil }),
		wormhole.WithProviderConfig("fixture", types.ProviderConfig{}), wormhole.WithDefaultProvider("fixture"))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func collectFixture(t *testing.T, chunks []types.TextChunk) *wormhole.Wormhole {
	t.Helper()
	provider, err := wormholetest.NewScriptedProvider("fixture", wormholetest.Script{Operation: wormholetest.ScriptStream, Chunks: chunks})
	require.NoError(t, err)
	return collectClient(t, provider)
}

func TestStreamAndCollectCompleteResponse(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonStop
	tool := types.ToolCall{ID: "call", Name: "lookup", Arguments: map[string]any{"value": []any{"original"}}, ThoughtSignature: "tool signature"}
	client := collectFixture(t, []types.TextChunk{
		{ID: "id", Provider: "fixture", Model: "routed", Text: "hello", Refusal: "no", Thinking: &types.Thinking{Content: "reason", Signature: "first", Provider: "fixture"},
			Delta: &types.ChunkDelta{Content: "hello", Refusal: "no", Thinking: &types.Thinking{Content: "reason"}}},
		{Delta: &types.ChunkDelta{Content: " world", Refusal: " thanks", Thinking: &types.Thinking{Content: "ing", Signature: "last"}}, ToolCall: &tool, ToolCalls: []types.ToolCall{tool}, FinishReason: &stop,
			Usage: &types.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}},
		{Usage: &types.Usage{PromptTokens: 9, CompletionTokens: 7, TotalTokens: 17, CacheReadTokens: 3, CacheWriteTokens: 2, ReasoningTokens: 4}},
		{Usage: &types.Usage{}},
	})
	stream, result, err := client.Text().Model("requested").Prompt("test").StreamAndCollect(context.Background())
	require.NoError(t, err)
	for chunk := range stream {
		if chunk.Thinking != nil {
			chunk.Thinking.Content = "consumer mutation"
		}
		if chunk.Usage != nil {
			chunk.Usage.PromptTokens = 999
		}
		if len(chunk.ToolCalls) > 0 {
			chunk.ToolCalls[0].Arguments["value"] = "consumer mutation"
		}
	}
	got, err := result()
	require.NoError(t, err)
	require.Equal(t, &types.TextResponse{ID: "id", Provider: "fixture", Model: "routed", Text: "hello world", Refusal: "no thanks",
		Thinking: &types.Thinking{Content: "reasoning", Signature: "last", Provider: "fixture"}, ToolCalls: []types.ToolCall{tool}, FinishReason: stop,
		Usage: &types.Usage{PromptTokens: 10, CompletionTokens: 7, TotalTokens: 17, CacheReadTokens: 3, CacheWriteTokens: 2, ReasoningTokens: 4}}, got)
	got.ToolCalls[0].Arguments["value"].([]any)[0] = "changed"
	got.Thinking.Content = "changed"
	got.Usage.PromptTokens = 999
	again, err := result()
	require.NoError(t, err)
	require.Equal(t, "original", again.ToolCalls[0].Arguments["value"].([]any)[0])
	require.Equal(t, "reasoning", again.Thinking.Content)
	require.Equal(t, 10, again.Usage.PromptTokens)
}

func TestStreamAndCollectRawTools(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonToolCalls
	encoded, err := json.Marshal(map[string]any{"city": "Paris"})
	require.NoError(t, err)
	raw := string(encoded)
	client := collectFixture(t, []types.TextChunk{
		{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{Index: 1, ID: "a", Function: &types.ToolCallFunction{Name: "weather", Arguments: raw[:5]}, ThoughtSignature: "signature"}}}},
		{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{Index: 2, ID: "b", Function: &types.ToolCallFunction{Name: "broken", Arguments: "{"}}}}},
		{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{Index: 1, Function: &types.ToolCallFunction{Arguments: raw[5:]}}}}, FinishReason: &stop},
	})
	stream, result, err := client.Text().Model("test-model").Prompt("test").StreamAndCollect(context.Background())
	require.NoError(t, err)
	for range stream {
	}
	got, err := result()
	require.NoError(t, err)
	require.Len(t, got.ToolCalls, 2)
	require.Equal(t, map[string]any{"city": "Paris"}, got.ToolCalls[0].Arguments)
	require.Equal(t, "weather", got.ToolCalls[0].Name)
	require.Equal(t, "signature", got.ToolCalls[0].ThoughtSignature)
	require.Equal(t, raw, got.ToolCalls[0].Function.Arguments)
	require.True(t, got.ToolCalls[1].ArgsInvalid)
	require.NotEmpty(t, got.ToolCalls[1].ArgsParseError)
	require.Equal(t, "{", got.ToolCalls[1].Function.Arguments)
}

func TestStreamAndCollectTerminalToolSnapshot(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonToolCalls
	complete := types.ToolCall{ID: "call", Name: "tool", Arguments: map[string]any{"done": true}, ArgsInvalid: true, ArgsParseError: "provider flag"}
	client := collectFixture(t, []types.TextChunk{
		{Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{{ID: "call", Function: &types.ToolCallFunction{Arguments: "{"}}}}},
		{ToolCalls: []types.ToolCall{complete}},
		{ToolCall: &complete, ToolCalls: []types.ToolCall{complete}, Delta: &types.ChunkDelta{ToolCalls: []types.ToolCall{complete}}, FinishReason: &stop},
	})
	stream, result, err := client.Text().Model("test-model").Prompt("test").StreamAndCollect(context.Background())
	require.NoError(t, err)
	for range stream {
	}
	got, err := result()
	require.NoError(t, err)
	require.Equal(t, []types.ToolCall{complete}, got.ToolCalls)
}

func TestStreamAndCollectErrorsAndEmptyCompletion(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonStop
	failure := errors.New("stream failure")
	cases := []struct {
		name   string
		chunks []types.TextChunk
		want   error
		text   string
	}{
		{"error data", []types.TextChunk{{Text: "prefix"}, {Text: " partial", Error: failure}}, failure, "prefix partial"},
		{"trailing error", []types.TextChunk{{Text: "prefix", FinishReason: &stop}, {Error: failure}}, failure, "prefix"},
		{"empty complete", []types.TextChunk{{FinishReason: &stop}}, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := collectFixture(t, tc.chunks)
			stream, result, err := client.Text().Model("requested").Prompt("test").StreamAndCollect(context.Background())
			require.NoError(t, err)
			for range stream {
			}
			got, err := result()
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, tc.text, got.Text)
			require.Equal(t, "requested", got.Model)
		})
	}
}

func TestStreamAndCollectSetupError(t *testing.T) {
	t.Parallel()
	client := collectFixture(t, nil)
	stream, result, err := client.Text().Prompt("test").StreamAndCollect(context.Background())
	require.Error(t, err)
	require.Nil(t, stream)
	require.Nil(t, result)
}

func TestStreamAndCollectCancellation(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonStop
	client := collectFixture(t, []types.TextChunk{{Text: "prefix"}, {Text: "next", FinishReason: &stop}})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stream, result, err := client.Text().Model("requested").Prompt("test").StreamAndCollect(ctx)
	require.NoError(t, err)
	require.Equal(t, "prefix", (<-stream).Text)
	cancel()
	for range stream {
	}
	got, err := result()
	require.ErrorIs(t, err, context.Canceled)
	require.Contains(t, got.Text, "prefix")
}

type collectClosureProvider struct {
	*types.BaseProvider
	never bool
}

func (p *collectClosureProvider) Stream(context.Context, types.TextRequest) (<-chan types.TextChunk, error) {
	stream := make(chan types.TextChunk, 1)
	if !p.never {
		stream <- types.TextChunk{}
		close(stream)
	}
	return stream, nil
}

func TestStreamAndCollectAbandonedReceiver(t *testing.T) {
	t.Parallel()
	client := collectFixture(t, []types.TextChunk{{Text: "pending"}})
	ctx, cancel := context.WithCancel(context.Background())
	_, result, err := client.Text().Model("requested").Prompt("test").StreamAndCollect(ctx)
	require.NoError(t, err)
	cancel()
	_, err = result()
	require.ErrorIs(t, err, context.Canceled)
}

func TestStreamAndCollectRepeatedCompletedTools(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonStop
	first := types.ToolCall{ID: "call", Name: "tool", Arguments: map[string]any{"value": "first"}}
	latest := types.ToolCall{ID: "call", Name: "tool", Arguments: map[string]any{"value": "latest"}}
	client := collectFixture(t, []types.TextChunk{{ToolCall: &first}, {ToolCall: &latest}, {FinishReason: &stop}})
	stream, result, err := client.Text().Model("requested").Prompt("test").StreamAndCollect(context.Background())
	require.NoError(t, err)
	for range stream {
	}
	got, err := result()
	require.NoError(t, err)
	require.Equal(t, []types.ToolCall{latest}, got.ToolCalls)
}

func TestStreamAndCollectProviderValidatedClosure(t *testing.T) {
	t.Parallel()
	client := collectClient(t, &collectClosureProvider{BaseProvider: types.NewBaseProvider("fixture")})
	stream, result, err := client.Text().Model("requested").Prompt("test").StreamAndCollect(context.Background())
	require.NoError(t, err)
	for range stream {
	}
	got, err := result()
	require.NoError(t, err)
	require.Empty(t, got.FinishReason)
}

func TestStreamAndCollectCancellationBeforeFirstChunk(t *testing.T) {
	t.Parallel()
	client := collectClient(t, &collectClosureProvider{BaseProvider: types.NewBaseProvider("fixture"), never: true})
	ctx, cancel := context.WithCancel(context.Background())
	stream, result, err := client.Text().Model("requested").Prompt("test").StreamAndCollect(ctx)
	require.NoError(t, err)
	cancel()
	for range stream {
	}
	got, err := result()
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, got.Text)
}
