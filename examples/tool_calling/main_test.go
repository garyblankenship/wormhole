package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
	"github.com/garyblankenship/wormhole/v3/wormholetest"
)

func TestExecuteManualToolRejectsMalformedArguments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		handlers map[string]types.ToolHandler
		call     types.ToolCall
		want     string
	}{
		{
			name: "provider argument parse failure",
			handlers: map[string]types.ToolHandler{
				"get_current_time": func(context.Context, map[string]any) (any, error) {
					return nil, errors.New("handler started for malformed provider arguments")
				},
			},
			call: types.ToolCall{
				ID:             "call-malformed",
				Name:           "get_current_time",
				ArgsInvalid:    true,
				ArgsParseError: "unexpected end of JSON input",
			},
			want: "malformed arguments",
		},
		{
			name:     "handler input validation failure",
			handlers: map[string]types.ToolHandler{"get_current_time": getCurrentTime},
			call: types.ToolCall{
				ID:        "call-invalid-type",
				Name:      "get_current_time",
				Arguments: map[string]any{"timezone": 42},
			},
			want: "timezone must be a non-empty string",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result := executeManualTool(context.Background(), test.handlers, test.call)
			if result.ToolCallID != test.call.ID || result.FunctionName != test.call.Name {
				t.Fatalf("correlation = (%q, %q), want (%q, %q)", result.ToolCallID, result.FunctionName, test.call.ID, test.call.Name)
			}
			if !strings.Contains(result.Error, test.want) || !strings.Contains(result.Content, test.want) {
				t.Fatalf("manual error result = %#v, want %q", result, test.want)
			}
		})
	}
}

func TestContinueManualToolCallsNormalizesBeforeProviderRequest(t *testing.T) {
	t.Parallel()

	mock := wormholetest.NewMockProvider("mock").
		WithTextResponse(types.TextResponse{Text: "continued"})
	client := wormhole.New(
		wormhole.WithCustomProvider("mock", wormholetest.MockProviderFactory(mock)),
		wormhole.WithProviderConfig("mock", types.ProviderConfig{}),
		wormhole.WithDefaultProvider("mock"),
		wormhole.WithDiscovery(false),
	)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	})

	handlers := map[string]types.ToolHandler{"get_current_time": getCurrentTime}
	client.RegisterTool(
		"get_current_time",
		"Get the current time in a specified timezone.",
		map[string]any{"type": "object"},
		handlers["get_current_time"],
	)

	_, err := continueManualToolCalls(context.Background(), client, handlers, "time?", &types.TextResponse{
		ToolCalls: []types.ToolCall{{
			ID:             "call-malformed",
			Name:           "get_current_time",
			ArgsInvalid:    true,
			ArgsParseError: "unexpected end of JSON input",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "normalize tool call") {
		t.Fatalf("malformed continuation error = %v", err)
	}

	response, err := continueManualToolCalls(context.Background(), client, handlers, "time?", &types.TextResponse{
		ToolCalls: []types.ToolCall{{
			ID:        "call-valid",
			Name:      "get_current_time",
			Arguments: map[string]any{"timezone": "Asia/Tokyo"},
		}},
	})
	if err != nil {
		t.Fatalf("continue manual tool calls: %v", err)
	}
	if response.Text != "continued" {
		t.Fatalf("continued response = %q, want continued", response.Text)
	}
}
