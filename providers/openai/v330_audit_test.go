package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/stretchr/testify/require"
)

func TestV330ResponsesMalformedAnnotationsPreserveOutput(t *testing.T) {
	t.Parallel()
	for _, annotations := range []any{"bad", []any{nil, "bad", 7, map[string]any{"type": "url_citation", "url": "https://example.com"}}} {
		t.Run(fmt.Sprint(annotations), func(t *testing.T) {
			t.Parallel()
			provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "usable", "annotations": annotations}}}}}))
			})
			provider.Config.UseResponsesAPI = true
			out, err := provider.Text(context.Background(), types.TextRequest{BaseRequest: types.BaseRequest{Model: "test"}, Messages: []types.Message{types.NewUserMessage("hello")}})
			require.NoError(t, err)
			require.Equal(t, "usable", out.Text)
			require.NotEmpty(t, out.Warnings)
			require.NotEmpty(t, out.Metadata["unparsed_annotations"])
			if _, ok := annotations.([]any); ok {
				require.Len(t, out.Sources, 1)
			}
		})
	}
}

func TestV330ResponsesReasoningSummary(t *testing.T) {
	t.Parallel()
	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, "auto", payload["reasoning"].(map[string]any)["summary"])
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"output_text": "answer", "output": []any{map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "first "}, map[string]any{"type": "summary_text", "text": "second"}}}}}))
	})
	provider.Config.UseResponsesAPI = true
	yes := true
	out, err := provider.Text(context.Background(), types.TextRequest{BaseRequest: types.BaseRequest{Model: "test", Reasoning: &types.Reasoning{IncludeThoughts: &yes}}, Messages: []types.Message{types.NewUserMessage("hello")}})
	require.NoError(t, err)
	require.NotNil(t, out.Thinking)
	require.Equal(t, "first second", out.Thinking.Content)
	require.Equal(t, "answer", out.Text)
}

func TestV330ResponsesPrewarmNumericAndMergedOptions(t *testing.T) {
	t.Parallel()
	for _, zero := range []any{json.Number("0"), int32(0), uint(0), float32(0)} {
		t.Run(fmt.Sprintf("%T", zero), func(t *testing.T) {
			t.Parallel()
			provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				if v, ok := payload["max_output_tokens"]; ok {
					require.Equal(t, float64(0), v)
				}
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"status": "completed"}))
			})
			provider.Config.UseResponsesAPI = true
			provider.Config.DefaultProviderOptions = map[string]any{"max_output_tokens": zero}
			request := types.TextRequest{BaseRequest: types.BaseRequest{Model: "test"}, Messages: []types.Message{types.NewUserMessage("warm")}}
			_, err := provider.Text(context.Background(), request)
			require.NoError(t, err)
			provider.Config.DefaultProviderOptions = nil
			request.ProviderOptions = map[string]any{"max_output_tokens": zero}
			_, err = provider.Text(context.Background(), request)
			require.NoError(t, err)
			request.ProviderOptions = nil
			_, err = provider.Text(context.Background(), request)
			require.Error(t, err)
		})
	}
}

func TestV330ResponsesStreamSourceIdentity(t *testing.T) {
	t.Parallel()
	a := map[string]any{"type": "url_citation", "url": "https://a.example"}
	b := map[string]any{"type": "url_citation", "url": "https://b.example"}
	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Arrive out of traversal order; terminal annotations must retain IDs.
		events := []any{
			map[string]any{"type": "response.output_text.annotation.added", "item_id": "b", "output_index": 1, "content_index": 0, "annotation_index": 0, "annotation": b},
			map[string]any{"type": "response.output_text.annotation.added", "item_id": "a", "output_index": 0, "content_index": 1, "annotation_index": 0, "annotation": a},
			map[string]any{"type": "response.output_text.annotation.added", "item_id": "a", "output_index": 0, "content_index": 1, "annotation_index": 0, "annotation": a},
			map[string]any{"type": "response.output_text.annotation.added", "annotation": "malformed"},
			map[string]any{"type": "response.completed", "response": map[string]any{"output": []any{
				map[string]any{"id": "a", "type": "message", "content": []any{map[string]any{"type": "output_text", "text": "answer", "annotations": []any{a}}, map[string]any{"type": "output_text", "annotations": []any{a}}}},
				map[string]any{"id": "b", "type": "message", "content": []any{map[string]any{"type": "output_text", "annotations": []any{b}}}},
			}}},
		}
		for _, event := range events {
			data, err := json.Marshal(event)
			require.NoError(t, err)
			_, err = fmt.Fprintf(w, "data: %s\n\n", data)
			require.NoError(t, err)
		}
	})
	provider.Config.UseResponsesAPI = true
	// Reuse the provider: state must reset for each response.
	for range 2 {
		stream, err := provider.Stream(context.Background(), types.TextRequest{BaseRequest: types.BaseRequest{Model: "test"}, Messages: []types.Message{types.NewUserMessage("hello")}})
		require.NoError(t, err)
		var sources []types.Source
		var citations []types.Citation
		var warnings []types.ResponseWarning
		var terminal *types.TextChunk
		for chunk := range stream {
			require.NoError(t, chunk.Error)
			if chunk.FinishReason != nil {
				snapshot := chunk
				terminal = &snapshot
				continue
			}
			sources = append(sources, chunk.Sources...)
			citations = append(citations, chunk.Citations...)
			warnings = append(warnings, chunk.Warnings...)
		}
		require.Len(t, sources, 2)
		require.Len(t, citations, 2)
		require.NotEmpty(t, warnings)
		require.Equal(t, "https://b.example", sources[0].URL)
		require.NotNil(t, terminal)
		require.Len(t, terminal.Sources, 3)
		require.Len(t, terminal.Citations, 3)
		finalSources := map[string]types.Source{}
		for _, source := range terminal.Sources {
			finalSources[source.ID] = source
		}
		for _, citation := range terminal.Citations {
			_, exists := finalSources[citation.SourceID]
			require.True(t, exists)
		}
		for index, source := range sources {
			require.Equal(t, fmt.Sprintf("source_%d", index+1), source.ID)
			require.Equal(t, source.ID, citations[index].SourceID)
			require.Equal(t, source.URL, finalSources[source.ID].URL)
		}
	}
}
