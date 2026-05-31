package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderResponsesAPIText(t *testing.T) {
	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/responses", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get(types.HeaderAuthorization))

		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "gpt-5", req["model"])
		assert.Equal(t, float64(128), req["max_output_tokens"])
		assert.Equal(t, map[string]any{"format": map[string]any{"type": "json_object"}}, req["text"])

		input := req["input"].([]any)
		require.Len(t, input, 2)
		assert.Equal(t, map[string]any{"type": "message", "role": "system", "content": "be terse"}, input[0])
		assert.Equal(t, map[string]any{"type": "message", "role": "user", "content": "hi"}, input[1])

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(responsesResponse{
			ID:        "resp-1",
			CreatedAt: 100,
			Model:     "gpt-5",
			Status:    "completed",
			Output: []responsesOutputItem{{
				ID:     "msg-1",
				Type:   responsesItemMessage,
				Role:   "assistant",
				Status: "completed",
				Content: []responsesContentPart{{
					Type: responsesContentOutputText,
					Text: `{"ok":true}`,
				}},
			}},
			Usage: responsesUsage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5},
		}))
	})
	provider.Config.UseResponsesAPI = true

	maxTokens := 128
	resp, err := provider.Text(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model:     "gpt-5",
			MaxTokens: &maxTokens,
		},
		Messages: []types.Message{
			types.NewSystemMessage("be terse"),
			types.NewUserMessage("hi"),
		},
		ResponseFormat: map[string]string{"type": "json_object"},
	})
	require.NoError(t, err)
	assert.Equal(t, "resp-1", resp.ID)
	assert.Equal(t, `{"ok":true}`, resp.Text)
	require.NotNil(t, resp.Usage)
	assert.Equal(t, 5, resp.Usage.TotalTokens)
	assert.Equal(t, types.FinishReasonStop, resp.FinishReason)
}

func TestProviderResponsesAPIToolCalling(t *testing.T) {
	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		tools := req["tools"].([]any)
		require.Len(t, tools, 1)
		tool := tools[0].(map[string]any)
		assert.Equal(t, "function", tool["type"])
		assert.Equal(t, "lookup", tool["name"])
		assert.Equal(t, "Lookup records", tool["description"])
		assert.Equal(t, map[string]any{"type": "function", "name": "lookup"}, req["tool_choice"])

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(responsesResponse{
			ID:        "resp-tool",
			CreatedAt: 100,
			Model:     "gpt-5",
			Status:    "completed",
			Output: []responsesOutputItem{{
				ID:        "fc-1",
				Type:      responsesItemFunctionCall,
				CallID:    "call-1",
				Name:      "lookup",
				Arguments: `{"q":"ada"}`,
				Status:    "completed",
			}},
		}))
	})
	provider.Config.UseResponsesAPI = true

	resp, err := provider.Text(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-5"},
		Messages:    []types.Message{types.NewUserMessage("lookup ada")},
		Tools: []types.Tool{*types.NewTool("lookup", "Lookup records", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"q": map[string]any{"type": "string"},
			},
		})},
		ToolChoice: &types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: "lookup"},
	})
	require.NoError(t, err)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "call-1", resp.ToolCalls[0].ID)
	assert.Equal(t, "lookup", resp.ToolCalls[0].Name)
	assert.Equal(t, map[string]any{"q": "ada"}, resp.ToolCalls[0].Arguments)
	assert.Equal(t, types.FinishReasonToolCalls, resp.FinishReason)
}

func TestProviderResponsesAPIStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/responses", r.URL.Path)
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, true, req["stream"])

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, `data: {"type":"response.output_text.delta","delta":"hel"}`+"\n\n")
		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, `data: {"type":"response.output_text.delta","delta":"lo"}`+"\n\n")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, `data: {"type":"response.completed","response":{"id":"resp-stream","created_at":100,"model":"gpt-5","status":"completed","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	provider := New(types.ProviderConfig{
		APIKey:          "test-key",
		BaseURL:         server.URL,
		UseResponsesAPI: true,
	})

	stream, err := provider.Stream(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-5"},
		Messages:    []types.Message{types.NewUserMessage("hi")},
	})
	require.NoError(t, err)

	var text string
	var final *types.TextChunk
	for chunk := range stream {
		require.NoError(t, chunk.Error)
		text += chunk.Content()
		if chunk.IsDone() {
			c := chunk
			final = &c
		}
	}
	assert.Equal(t, "hello", text)
	require.NotNil(t, final)
	assert.Equal(t, "openai", final.Provider)
	assert.Equal(t, "resp-stream", final.ID)
	require.NotNil(t, final.Usage)
	assert.Equal(t, 3, final.Usage.TotalTokens)
}
