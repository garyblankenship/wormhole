package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

// C6-01: handleStream must clone refs.warnings[oldWarnings:] so downstream
// streamDetails appending warnings cannot race with or corrupt refs.warnings.
func TestRemediationC601_StreamWarningsCloned(t *testing.T) {
	t.Parallel()

	// Two frames: first emits a citation warning (unmappable range / empty URI),
	// second emits finishReason. With Reasoning.Enabled set, streamDetails appends
	// a reasoning warning to chunk.Warnings on the first and final chunks.
	frame1 := `data: {"candidates":[{"content":{"parts":[{"text":"hello"}]},"groundingMetadata":{"groundingChunks":[{"web":{"uri":""}}],"groundingSupports":[{"segment":{"startIndex":0,"endIndex":5},"groundingChunkIndices":[0]}]}}]}` + "\n\n"
	frame2 := `data: {"candidates":[{"content":{"parts":[{"text":" world"}]},"finishReason":"STOP"}]}` + "\n\n"
	sse := frame1 + frame2 + "data: [DONE]\n\n"

	g := New("test-key", types.ProviderConfig{})
	enabled := true
	req := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:     "gemini-2.5-flash",
			Reasoning: &types.Reasoning{Enabled: &enabled},
		},
	}

	ctx := context.Background()
	stream := g.streamDetails(
		ctx,
		g.stampProvider(ctx, g.handleStream(ctx, io.NopCloser(strings.NewReader(sse)))),
		&types.ResponseMetadata{RequestID: "req-1"},
		req,
	)

	var chunks []types.TextChunk
	for ch := range stream {
		require.NoError(t, ch.Error)
		chunks = append(chunks, ch)
	}
	require.GreaterOrEqual(t, len(chunks), 2)
}

// C6-04: schemaToMap must handle json.RawMessage as well as []byte and map[string]any.
func TestRemediationC604_SchemaToMapRawMessage(t *testing.T) {
	t.Parallel()
	g := New("test-key", types.ProviderConfig{})

	raw := json.RawMessage(`{"type":"object","properties":{"name":{"type":["string","null"]}},"required":["name"]}`)
	got := g.schemaToMap(raw)
	assert.Equal(t, "object", got["type"])
	props, ok := got["properties"].(map[string]any)
	require.True(t, ok)
	nameProp, ok := props["name"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", nameProp["type"])
	assert.Equal(t, true, nameProp["nullable"])
}

// C6-05: ToolResultMessage followed by UserMessage (adjacent non-model turns)
// must coalesce into a single non-model turn instead of emitting adjacent
// function + user turns.
func TestRemediationC605_CoalesceToolResultAndUserMessage(t *testing.T) {
	t.Parallel()
	g := New("test-key", types.ProviderConfig{})

	msgs := []types.Message{
		types.NewUserMessage("What is the weather?"),
		&types.AssistantMessage{
			ToolCalls: []types.ToolCall{
				{ID: "call-1", Name: "get_weather", Arguments: map[string]any{"city": "NYC"}},
			},
		},
		&types.ToolResultMessage{
			ToolCallID:   "call-1",
			FunctionName: "get_weather",
			Content:      `{"temp":72}`,
		},
		types.NewUserMessage("And tomorrow?"),
	}

	contents, err := g.transformMessages(msgs, "gemini-2.5-flash")
	require.NoError(t, err)
	require.Len(t, contents, 3, "tool result + trailing user message must coalesce")
	assert.Equal(t, "user", contents[0]["role"])
	assert.Equal(t, "model", contents[1]["role"])
	assert.Equal(t, "user", contents[2]["role"])

	parts, ok := contents[2]["parts"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, parts, 2)
	assert.Contains(t, parts[0], "functionResponse")
	assert.Equal(t, "And tomorrow?", parts[1]["text"])
}

// C6-07: Normalized model name for gemini-3 thoughtSignature sentinel, plus
// fallback to toolCall.Function.Name / toolCall.Function.Arguments.
func TestRemediationC607_NormalizedModelSentinelAndFunctionFallback(t *testing.T) {
	t.Parallel()
	g := New("test-key", types.ProviderConfig{})

	for _, model := range []string{"models/gemini-3-pro-preview", "google/gemini-3.6-flash", "google/models/gemini-3-pro"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			payload, err := g.buildTextPayload(types.TextRequest{
				BaseRequest: types.BaseRequest{Model: model},
				Messages: []types.Message{
					types.NewUserMessage("look up go"),
					&types.AssistantMessage{
						ToolCalls: []types.ToolCall{
							{
								ID: "call-1",
								Function: &types.ToolCallFunction{
									Name:      "lookup",
									Arguments: `{"q":"go"}`,
								},
							},
						},
					},
					&types.ToolResultMessage{
						ToolCallID:   "call-1",
						FunctionName: "lookup",
						Content:      `{"ok":true}`,
					},
				},
			})
			require.NoError(t, err)
			contents := payload["contents"].([]map[string]any)
			require.Len(t, contents, 3)
			parts := contents[1]["parts"].([]map[string]any)
			require.Len(t, parts, 1)
			assert.Equal(t, geminiThoughtSignatureSentinel, parts[0]["thoughtSignature"])
			fc := parts[0]["functionCall"].(map[string]any)
			assert.Equal(t, "lookup", fc["name"])
			assert.Equal(t, map[string]any{"q": "go"}, fc["args"])
		})
	}
}

// C6-09: Structured sets Model, Created, preserves citation/grounding metadata,
// and validates map/[]byte/json.RawMessage schemas.
func TestRemediationC609_StructuredMetadataAndSchemaValidation(t *testing.T) {
	t.Parallel()

	t.Run("sets Model, Created, and preserves grounding/citation metadata", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"candidates": [{
					"content": {"parts": [{"text": "{\"name\":\"Ada\"}"}]},
					"finishReason": "STOP",
					"citationMetadata": {"citationSources": [{"uri": "https://example.com"}]},
					"groundingMetadata": {"webSearchQueries": ["ada"]}
				}],
				"usageMetadata": {"promptTokenCount": 5, "candidatesTokenCount": 3, "totalTokenCount": 8}
			}`))
		}))
		t.Cleanup(server.Close)

		g := New("test-key", types.ProviderConfig{BaseURL: server.URL})
		resp, err := g.Structured(context.Background(), types.StructuredRequest{
			BaseRequest: types.BaseRequest{Model: "gemini-2.5-flash"},
			Messages:    []types.Message{types.NewUserMessage("extract")},
			Mode:        types.StructuredModeStrict,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
				},
				"required": []any{"name"},
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "gemini-2.5-flash", resp.Model)
		assert.False(t, resp.Created.IsZero())
		assert.Contains(t, resp.Metadata, "citationMetadata")
		assert.Contains(t, resp.Metadata, "groundingMetadata")
	})

	t.Run("validates map, []byte, and json.RawMessage schemas", func(t *testing.T) {
		t.Parallel()
		g := New("test-key", types.ProviderConfig{})
		rawResp := &geminiTextResponse{
			Candidates: []candidate{{
				Content:      content{Parts: []part{{Text: `{"age":"not-a-number"}`}}},
				FinishReason: "STOP",
			}},
		}

		schemaMap := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"age": map[string]any{"type": "integer"},
			},
			"required": []any{"age"},
		}
		schemaBytes, err := json.Marshal(schemaMap)
		require.NoError(t, err)

		for _, tc := range []struct {
			name   string
			schema types.Schema
		}{
			{"map", schemaMap},
			{"bytes", schemaBytes},
			{"rawMessage", json.RawMessage(schemaBytes)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				_, err := g.transformStructuredResponse(rawResp, tc.schema)
				require.Error(t, err, "invalid structured output must fail validation for %s schema", tc.name)
			})
		}
	})
}
