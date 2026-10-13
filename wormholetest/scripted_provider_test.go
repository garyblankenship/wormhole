package wormholetest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestScriptedProviderOrdering(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonStop
	failure := errors.New("scripted failure")
	provider, err := NewScriptedProvider("fixture",
		Script{Operation: ScriptText, Response: &types.TextResponse{Text: "first"}},
		Script{Operation: ScriptStream, Chunks: []types.TextChunk{{Text: "second", FinishReason: &stop}, {Usage: &types.Usage{PromptTokens: 4}}}},
		Script{Operation: ScriptText, Error: failure})
	require.NoError(t, err)
	ctx := context.Background()
	_, err = provider.Stream(ctx, types.TextRequest{})
	require.ErrorIs(t, err, ErrScriptMismatch)
	_, err = provider.Structured(ctx, types.StructuredRequest{})
	require.Error(t, err)
	response, err := provider.Text(ctx, types.TextRequest{})
	require.NoError(t, err)
	require.Equal(t, "first", response.Text)
	stream, err := provider.Stream(ctx, types.TextRequest{})
	require.NoError(t, err)
	chunks := drainScript(stream)
	require.Len(t, chunks, 2)
	require.Equal(t, 4, chunks[1].Usage.PromptTokens)
	_, err = provider.Text(ctx, types.TextRequest{})
	require.ErrorIs(t, err, failure)
	_, err = provider.Text(ctx, types.TextRequest{})
	require.ErrorIs(t, err, ErrScriptExhausted)
	require.Equal(t, []types.ModelCapability{types.CapabilityText, types.CapabilityChat, types.CapabilityStream}, provider.SupportedCapabilities())
	require.NoError(t, provider.Close())
}

func TestScriptedProviderCancellation(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonStop
	provider, err := NewScriptedProvider("fixture", Script{Operation: ScriptStream, Chunks: []types.TextChunk{{Text: "prefix"}, {FinishReason: &stop}}})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.Stream(ctx, types.TextRequest{})
	require.ErrorIs(t, err, context.Canceled)
	ctx, cancel = context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stream, err := provider.Stream(ctx, types.TextRequest{})
	require.NoError(t, err)
	first := <-stream
	require.Equal(t, "prefix", first.Text)
	cancel()
	for range stream {
	}
	_, err = provider.Stream(context.Background(), types.TextRequest{})
	require.ErrorIs(t, err, ErrScriptExhausted)
}

func TestScriptedProviderDeliveredCompletionOwnership(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonStop
	provider, err := NewScriptedProvider("fixture", Script{Operation: ScriptStream, Chunks: []types.TextChunk{
		{FinishReason: &stop},
		{Usage: &types.Usage{PromptTokens: 1}},
	}})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stream, err := provider.Stream(ctx, types.TextRequest{})
	require.NoError(t, err)
	completed, ok := <-stream
	require.True(t, ok)
	require.NotNil(t, completed.FinishReason)
	// The receive transfers ownership. Mutate before allowing the next send;
	// the race gate also checks there are no producer reads after this handoff.
	*completed.FinishReason = ""
	trailer, ok := <-stream
	require.True(t, ok)
	require.Equal(t, 1, trailer.Usage.PromptTokens)
	_, ok = <-stream
	require.False(t, ok, "receiver mutation must not erase completion and add EOF")
}

func TestScriptedProviderChunkErrorsAndEOF(t *testing.T) {
	t.Parallel()
	failure := errors.New("chunk failure")
	stop := types.FinishReasonStop
	cases := []struct {
		name   string
		chunks []types.TextChunk
		want   error
		count  int
	}{
		{"missing completion", []types.TextChunk{{Text: "prefix"}}, io.ErrUnexpectedEOF, 2},
		{"empty malformed", nil, io.ErrUnexpectedEOF, 1},
		{"partial error", []types.TextChunk{{Text: "prefix"}, {Error: failure}, {Text: "unreachable"}}, failure, 2},
		{"error after finish", []types.TextChunk{{FinishReason: &stop}, {Error: failure}}, failure, 2},
		{"empty completion", []types.TextChunk{{FinishReason: &stop}}, nil, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider, err := NewScriptedProvider("fixture", Script{Operation: ScriptStream, Chunks: tc.chunks})
			require.NoError(t, err)
			stream, err := provider.Stream(context.Background(), types.TextRequest{})
			require.NoError(t, err)
			chunks := drainScript(stream)
			require.Len(t, chunks, tc.count)
			require.ErrorIs(t, chunks[len(chunks)-1].Error, tc.want)
		})
	}
}

func TestScriptedProviderFixtureIsolation(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonStop
	response := &types.TextResponse{Text: "original", Metadata: map[string]any{"value": []any{"original"}}}
	chunks := []types.TextChunk{{Thinking: &types.Thinking{Content: "original"}, FinishReason: &stop}}
	provider, err := NewScriptedProvider("fixture", Script{Operation: ScriptText, Response: response}, Script{Operation: ScriptText, Response: response}, Script{Operation: ScriptStream, Chunks: chunks})
	require.NoError(t, err)
	response.Text = "changed"
	response.Metadata["value"].([]any)[0] = "changed"
	chunks[0].Thinking.Content = "changed"
	stop = types.FinishReasonLength
	first, err := provider.Text(context.Background(), types.TextRequest{})
	require.NoError(t, err)
	require.Equal(t, "original", first.Text)
	first.Metadata["value"].([]any)[0] = "result changed"
	second, err := provider.Text(context.Background(), types.TextRequest{})
	require.NoError(t, err)
	require.Equal(t, "original", second.Metadata["value"].([]any)[0])
	stream, err := provider.Stream(context.Background(), types.TextRequest{})
	require.NoError(t, err)
	got := drainScript(stream)
	require.Equal(t, "original", got[0].Thinking.Content)
	require.Equal(t, types.FinishReasonStop, *got[0].FinishReason)
}

func TestScriptedProviderConcurrentReservation(t *testing.T) {
	t.Parallel()
	const count = 32
	scripts := make([]Script, count)
	for i := range scripts {
		scripts[i] = Script{Operation: ScriptText, Response: &types.TextResponse{ID: fmt.Sprint(i)}}
	}
	provider, err := NewScriptedProvider("fixture", scripts...)
	require.NoError(t, err)
	type result struct {
		response *types.TextResponse
		err      error
	}
	results := make(chan result, count)
	for range count {
		// Exit after one atomic reservation and one buffered result send.
		go func() {
			response, callErr := provider.Text(context.Background(), types.TextRequest{})
			results <- result{response, callErr}
		}()
	}
	seen := make(map[string]bool)
	for range count {
		got := <-results
		require.NoError(t, got.err)
		require.False(t, seen[got.response.ID])
		seen[got.response.ID] = true
	}
	require.Len(t, seen, count)
	_, err = provider.Text(context.Background(), types.TextRequest{})
	require.ErrorIs(t, err, ErrScriptExhausted)
}

func TestScriptedProviderValidation(t *testing.T) {
	t.Parallel()
	failure := errors.New("fixture")
	cases := []Script{
		{Operation: "unknown"},
		{Operation: ScriptText},
		{Operation: ScriptText, Response: &types.TextResponse{}, Chunks: []types.TextChunk{{}}},
		{Operation: ScriptStream, Response: &types.TextResponse{}},
		{Operation: ScriptText, Response: &types.TextResponse{}, Error: failure},
		{Operation: ScriptStream, Chunks: []types.TextChunk{{}}, Error: failure},
	}
	for i, script := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			_, err := NewScriptedProvider("fixture", script)
			require.Error(t, err)
		})
	}
	_, err := NewScriptedProvider(" ")
	require.Error(t, err)
	_, err = ScriptedProviderFactory(nil)(types.ProviderConfig{})
	require.Error(t, err)
	provider, err := NewScriptedProvider("fixture", Script{Operation: ScriptStream, Error: failure})
	require.NoError(t, err)
	_, err = provider.Stream(context.Background(), types.TextRequest{})
	require.ErrorIs(t, err, failure)
}

func drainScript(stream <-chan types.TextChunk) []types.TextChunk {
	var chunks []types.TextChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}
	return chunks
}
