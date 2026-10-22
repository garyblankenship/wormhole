package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/stretchr/testify/require"
)

func TestV330ResponsesMetadataUsagePrewarm(t *testing.T) {
	t.Parallel()
	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, float64(0), payload["max_output_tokens"])
		w.Header().Set("X-Request-ID", "request-v330")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"id": "prewarm", "status": "completed", "usage": map[string]any{"input_tokens": 12, "output_tokens": 0, "total_tokens": 12, "input_tokens_details": map[string]any{"cached_tokens": 7}, "future_detail": true}}))
	})
	provider.Config.UseResponsesAPI = true
	response, err := provider.Text(context.Background(), types.TextRequest{BaseRequest: types.BaseRequest{Model: "test", ProviderOptions: map[string]any{"max_output_tokens": 0}}, Messages: []types.Message{types.NewUserMessage("warm")}})
	require.NoError(t, err)
	require.Empty(t, response.Text)
	require.Equal(t, 7, response.Usage.CacheReadTokens)
	require.Equal(t, 12, response.Usage.TotalTokens)
	require.Equal(t, "request-v330", response.ResponseMetadata.RequestID)
	require.Equal(t, true, response.Metadata["provider_usage"].(map[string]any)["future_detail"])
}

func TestV330ReasoningAndFileReferences(t *testing.T) {
	t.Parallel()
	provider := New(types.NewProviderConfig("key"))
	provider.Config.UseResponsesAPI = true
	no := false
	req := types.TextRequest{BaseRequest: types.BaseRequest{Model: "test", Reasoning: &types.Reasoning{Effort: types.ReasoningEffortNone, IncludeThoughts: &no}}, Messages: []types.Message{&types.UserMessage{Media: []types.Media{&types.FileReferenceMedia{Provider: "openai", Kind: "document", ID: "file_123"}}}}}
	require.NoError(t, provider.validateV330Request(req))
	payload := provider.buildResponsesPayload(&req, req.Messages)
	require.Equal(t, "none", payload["reasoning"].(map[string]any)["effort"])
	input := payload["input"].([]map[string]any)
	require.Equal(t, map[string]any{"type": "input_file", "file_id": "file_123"}, input[0]["content"].([]map[string]any)[0])
	req.Reasoning.Enabled = &no
	req.Reasoning.MaxTokens = 10
	require.Error(t, provider.validateV330Request(req))
	req.Reasoning = nil
	req.Messages[0].(*types.UserMessage).Media[0].(*types.FileReferenceMedia).Provider = "other"
	require.Error(t, provider.validateV330Request(req))
}

func TestV330ResponsesCitations(t *testing.T) {
	t.Parallel()
	p := New(types.NewProviderConfig("key"))
	var response responsesResponse
	require.NoError(t, json.Unmarshal([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"évidence","annotations":[{"type":"url_citation","url":"https://example.com","title":"A","start_index":0,"end_index":3},{"type":"file_citation"}]}]}]}`), &response))
	out := p.transformResponsesTextResponse(&response)
	require.Len(t, out.Sources, 1)
	require.Len(t, out.Citations, 1)
	require.Nil(t, out.Citations[0].OutputRange)
	require.Len(t, out.Warnings, 1)
	require.NotNil(t, out.Metadata["unparsed_annotations"])
}
