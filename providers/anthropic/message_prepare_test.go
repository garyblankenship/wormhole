package anthropic_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/garyblankenship/wormhole/v3/providers/anthropic"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestPrepareMessagesHardErrorPreventsAnthropicHTTP(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	t.Cleanup(server.Close)
	provider := anthropic.New(types.ProviderConfig{APIKey: "test-key", BaseURL: server.URL})
	_, err := provider.Text(context.Background(), types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "claude-sonnet-5"},
		Messages: []types.Message{&types.AssistantMessage{ToolCalls: []types.ToolCall{
			{ID: "duplicate", Name: "one"},
			{ID: "duplicate", Name: "two"},
		}}},
	})
	if err == nil {
		t.Fatal("Text accepted duplicate normalized tool-call IDs")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("HTTP calls after preparation error = %d, want 0", got)
	}
}
