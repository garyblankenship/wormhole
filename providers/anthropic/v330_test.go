package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestV330AnthropicUsageCitationsMetadata(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "anthropic-id")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "message", "model": "claude", "stop_reason": "end_turn", "usage": map[string]any{"input_tokens": 10, "output_tokens": 3, "cache_read_input_tokens": 20, "cache_creation_input_tokens": 5, "cache_creation": map[string]int{"ephemeral_5m_input_tokens": 5}}, "content": []any{map[string]any{"type": "text", "text": "évidence", "citations": []any{map[string]any{"type": "page_location", "document_index": 0, "document_title": "Record", "start_page_number": 1, "end_page_number": 2}}}}})
	}))
	t.Cleanup(server.Close)
	p := New(types.ProviderConfig{APIKey: "key", BaseURL: server.URL})
	response, err := p.Text(context.Background(), types.TextRequest{BaseRequest: types.BaseRequest{Model: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if response.ResponseMetadata == nil || response.ResponseMetadata.RequestID != "anthropic-id" {
		t.Fatalf("metadata=%+v", response.ResponseMetadata)
	}
	if response.Usage.PromptTokens != 35 || response.Usage.TotalTokens != 38 || response.Usage.CacheWriteTokensByTTL["ephemeral_5m_input_tokens"] != 5 {
		t.Fatalf("usage=%+v", response.Usage)
	}
	if len(response.Sources) != 1 || len(response.Citations) != 1 || response.Citations[0].OutputRange != nil || *response.Citations[0].SourceLocation.StartPage != 1 {
		t.Fatalf("citations=%+v", response.Citations)
	}
	if response.Metadata["provider_usage"] == nil {
		t.Fatal("missing original usage")
	}
}

func TestV330AnthropicControlsReferences(t *testing.T) {
	t.Parallel()
	p := New(types.ProviderConfig{APIKey: "key"})
	no := false
	request := types.TextRequest{BaseRequest: types.BaseRequest{Model: "claude", Reasoning: &types.Reasoning{Effort: types.ReasoningEffortHigh, IncludeThoughts: &no}}, Messages: []types.Message{&types.UserMessage{Content: "read", Media: []types.Media{&types.FileReferenceMedia{Provider: "anthropic", Kind: "document", ID: "file_1"}}}}}
	payload, err := p.buildMessagePayload(&request, request.Messages)
	if err != nil {
		t.Fatal(err)
	}
	thinking := payload["thinking"].(map[string]any)
	if thinking["type"] != "adaptive" || thinking["display"] != "omitted" || payload["output_config"].(map[string]any)["effort"] != "high" {
		t.Fatalf("payload=%+v", payload)
	}
	messages := payload["messages"].([]map[string]any)
	content := messages[0]["content"].([]map[string]any)
	if content[1]["source"].(map[string]any)["file_id"] != "file_1" {
		t.Fatalf("content=%+v", content)
	}
	request.Reasoning.Enabled = &no
	request.Reasoning.MaxTokens = 1024
	if p.validateSamplingControls(request) == nil {
		t.Fatal("accepted contradictory reasoning")
	}
	request.Reasoning = nil
	request.Messages[0].(*types.UserMessage).Media[0].(*types.FileReferenceMedia).Provider = "other"
	if _, err = p.buildMessagePayload(&request, request.Messages); err == nil {
		t.Fatal("accepted foreign file")
	}
}

func TestV330AnthropicCitationStream(t *testing.T) {
	t.Parallel()
	p := New(types.ProviderConfig{APIKey: "key"})
	in := make(chan types.StreamChunk, 3)
	for _, wire := range []string{
		`{"type":"content_block_delta","delta":{"type":"citations_delta","citation":{"type":"char_location","document_index":0,"start_char_index":1,"end_char_index":4}}}`,
		`{"type":"content_block_delta","delta":{"type":"citations_delta","citation":{"type":"char_location","document_index":0,"start_char_index":1,"end_char_index":4}}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
	} {
		chunk, err := p.parseStreamChunk([]byte(wire))
		if err != nil {
			t.Fatal(err)
		}
		in <- *chunk
	}
	close(in)
	var final types.StreamChunk
	for chunk := range p.accumulatingStream(context.Background(), in) {
		if chunk.IsDone() {
			final = chunk
		}
	}
	if len(final.Sources) != 1 || len(final.Citations) != 1 {
		t.Fatalf("snapshot=%+v", final)
	}
}

func TestV330AnthropicStructuredJSONAndVisibility(t *testing.T) {
	t.Parallel()
	no := false
	for _, toolTurn := range []bool{false, true} {
		t.Run(map[bool]string{false: "final", true: "tool"}[toolTurn], func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if payload["thinking"] != nil {
					t.Errorf("visibility-only introduced thinking: %+v", payload["thinking"])
				}
				output, _ := payload["output_config"].(map[string]any)
				format, _ := output["format"].(map[string]any)
				if format["type"] != "json_schema" || format["schema"] == nil {
					t.Errorf("missing native schema: %+v", payload)
				}
				content := []any{map[string]any{"type": "text", "text": `{"value":"ok"}`}}
				stop := "end_turn"
				if toolTurn {
					content = []any{map[string]any{"type": "tool_use", "id": "call_1", "name": "lookup", "input": map[string]any{"key": "a"}}}
					stop = "tool_use"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "result", "model": "claude", "stop_reason": stop, "content": content})
			}))
			t.Cleanup(server.Close)
			provider := New(types.ProviderConfig{APIKey: "key", BaseURL: server.URL})
			response, err := provider.Structured(context.Background(), types.StructuredRequest{
				BaseRequest: types.BaseRequest{Model: "claude", Reasoning: &types.Reasoning{IncludeThoughts: &no}},
				Mode:        types.StructuredModeJSON, Schema: map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string"}}, "required": []string{"value"}},
				Tools: []types.Tool{*types.NewTool("lookup", "Lookup", map[string]any{"type": "object"})},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(response.Warnings) != 1 || response.Warnings[0].Field != "reasoning.include_thoughts" {
				t.Fatalf("warnings=%+v", response.Warnings)
			}
			if toolTurn {
				if len(response.ToolCalls) != 1 || response.Data != nil {
					t.Fatalf("tool response=%+v", response)
				}
			} else if response.Data == nil || response.Raw == "" {
				t.Fatalf("final response=%+v", response)
			}
		})
	}
}
