package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestProviderResponsesAPIText(t *testing.T) {
	t.Parallel()
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

func TestProviderResponsesAPISerializesUserMediaAsInputImageParts(t *testing.T) {
	t.Parallel()
	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		input := req["input"].([]any)
		require.Len(t, input, 1)
		message := input[0].(map[string]any)
		assert.Equal(t, "message", message["type"])
		assert.Equal(t, "user", message["role"])
		parts := message["content"].([]any)
		require.Len(t, parts, 3)
		assert.Equal(t, map[string]any{"type": "input_text", "text": "compare these"}, parts[0])
		assert.Equal(t, map[string]any{"type": "input_image", "image_url": "data:image/png;base64,aW1hZ2U="}, parts[1])
		assert.Equal(t, map[string]any{"type": "input_image", "image_url": "https://example.test/image.jpg"}, parts[2])

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(responsesResponse{
			ID:        "resp-media",
			CreatedAt: 100,
			Model:     "gpt-5",
			Status:    "completed",
			Output: []responsesOutputItem{{
				Type:   responsesItemMessage,
				Role:   "assistant",
				Status: "completed",
				Content: []responsesContentPart{{
					Type: responsesContentOutputText,
					Text: "ok",
				}},
			}},
		}))
	})
	provider.Config.UseResponsesAPI = true

	_, err := provider.Text(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-5"},
		Messages: []types.Message{
			&types.UserMessage{
				Content: "compare these",
				Media: []types.Media{
					&types.ImageMedia{MimeType: "image/png", Base64Data: "aW1hZ2U="},
					&types.ImageMedia{URL: "https://example.test/image.jpg"},
				},
			},
		},
	})
	require.NoError(t, err)
}

func TestProviderResponsesAPIToolCalling(t *testing.T) {
	t.Parallel()
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

func TestProviderResponsesAPIPreparesMessages(t *testing.T) {
	t.Parallel()
	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		input := req["input"].([]any)
		require.Len(t, input, 3)
		user := input[0].(map[string]any)
		assert.Equal(t, "hello", user["content"])

		call := input[1].(map[string]any)
		callID, ok := call["call_id"].(string)
		require.True(t, ok)
		assert.NotEmpty(t, callID)
		assert.Equal(t, callID, call["id"])

		result := input[2].(map[string]any)
		assert.Equal(t, "tool result", result["output"])

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(responsesResponse{
			ID:        "resp-prepared",
			CreatedAt: 100,
			Model:     "gpt-5",
			Status:    "completed",
			Output: []responsesOutputItem{{
				Type:   responsesItemMessage,
				Role:   "assistant",
				Status: "completed",
				Content: []responsesContentPart{{
					Type: responsesContentOutputText,
					Text: "ok",
				}},
			}},
		}))
	})
	provider.Config.UseResponsesAPI = true

	_, err := provider.Text(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "gpt-5"},
		Messages: []types.Message{
			types.NewUserMessage("hel\xfflo"),
			&types.AssistantMessage{
				ToolCalls: []types.ToolCall{{
					ID:        "call-1",
					Name:      "lookup",
					Arguments: map[string]any{"q": "ada"},
				}},
			},
			types.NewToolResultMessage("call-1", "tool result"),
		},
	})
	require.NoError(t, err)
}

func TestProviderResponsesAPIStream(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/responses", r.URL.Path)
		var req map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, true, req["stream"])

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.output_text.delta\n")
		_, _ = fmt.Fprint(w, `data: {"type":"response.output_text.delta","delta":"hel"}`+"\n\n")
		_, _ = fmt.Fprint(w, "event: response.output_text.delta\n")
		_, _ = fmt.Fprint(w, `data: {"type":"response.output_text.delta","delta":"lo"}`+"\n\n")
		_, _ = fmt.Fprint(w, "event: response.completed\n")
		_, _ = fmt.Fprint(w, `data: {"type":"response.completed","response":{"id":"resp-stream","created_at":100,"model":"gpt-5","status":"completed","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`+"\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
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

func TestParseResponsesStreamFunctionCallAndThinkingEvents(t *testing.T) {
	t.Parallel()
	provider := New(types.ProviderConfig{APIKey: "test-key", UseResponsesAPI: true})

	start, err := provider.parseResponsesStreamChunk([]byte(`{"type":"response.output_item.added","item_id":"item-1","item":{"type":"function_call","call_id":"call-1","name":"lookup","arguments":""}}`))
	require.NoError(t, err)
	require.NotNil(t, start)
	require.Len(t, start.ToolCalls, 1)
	assert.Equal(t, "call-1", start.ToolCalls[0].ID)
	assert.Equal(t, "lookup", start.ToolCalls[0].Name)
	require.NotNil(t, start.Delta)
	require.Len(t, start.Delta.ToolCalls, 1)

	args, err := provider.parseResponsesStreamChunk([]byte(`{"type":"response.function_call_arguments.delta","item_id":"call-1","delta":"{\"q\""}`))
	require.NoError(t, err)
	require.NotNil(t, args)
	require.Len(t, args.ToolCalls, 1)
	require.NotNil(t, args.ToolCalls[0].Function)
	assert.Equal(t, `{"q"`, args.ToolCalls[0].Function.Arguments)

	thinking, err := provider.parseResponsesStreamChunk([]byte(`{"type":"response.reasoning_summary_text.delta","item_id":"rs-1","delta":"considering"}`))
	require.NoError(t, err)
	require.NotNil(t, thinking)
	require.NotNil(t, thinking.Thinking)
	assert.Equal(t, "considering", thinking.Thinking.Content)
	require.NotNil(t, thinking.Delta)
	require.NotNil(t, thinking.Delta.Thinking)
}
