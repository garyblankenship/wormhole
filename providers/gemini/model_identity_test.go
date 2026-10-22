package gemini

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/stretchr/testify/require"
)

func TestNativeTextAndStreamRetainRequestedModel(t *testing.T) {
	t.Parallel()
	model := "models/gemini-identity-fixture"
	textFrame := map[string]any{"candidates": []any{map[string]any{
		"content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": "identified"}}},
	}}}
	finishFrame := map[string]any{"candidates": []any{map[string]any{"finishReason": "STOP"}}}
	usageFrame := map[string]any{"usageMetadata": map[string]int{
		"promptTokenCount": 2, "candidatesTokenCount": 1, "totalTokenCount": 3,
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ":streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			for _, frame := range []any{textFrame, finishFrame, usageFrame} {
				data, err := json.Marshal(frame)
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
					t.Error(err)
					return
				}
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(textFrame); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	zero := 0
	provider := New("fixture-key", types.ProviderConfig{BaseURL: server.URL, MaxRetries: &zero})
	t.Cleanup(func() { require.NoError(t, provider.Close()) })
	request := types.TextRequest{
		BaseRequest: types.BaseRequest{Model: model},
		Messages:    []types.Message{types.NewUserMessage("identify the route")},
	}
	response, err := provider.Text(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, model, response.Model)
	require.Equal(t, "identified", response.Text)
	stream, err := provider.Stream(t.Context(), request)
	require.NoError(t, err)
	var text strings.Builder
	var usage *types.Usage
	terminal := 0
	for chunk := range stream {
		require.NoError(t, chunk.Error)
		require.Equal(t, model, chunk.Model)
		text.WriteString(chunk.Content())
		if chunk.IsDone() {
			terminal++
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	require.Equal(t, "identified", text.String())
	require.Equal(t, 1, terminal)
	require.NotNil(t, usage)
	require.Equal(t, 3, usage.TotalTokens)
}
