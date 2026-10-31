package wormhole

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

type countingToolOutput struct {
	calls *atomic.Int32
}

func (v countingToolOutput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]int{"call": int(v.calls.Add(1))})
}

func TestAutomaticToolLoopReusesValidatedJSON(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	executor := remediationToolExecutor(t, DefaultToolSafetyConfig(), map[string]types.ToolHandler{
		"tool": func(context.Context, map[string]any) (any, error) {
			return countingToolOutput{calls: &calls}, nil
		},
	})
	var turns int
	_, err := executor.executeWithTools(context.Background(), types.TextRequest{}, func(_ context.Context, req types.TextRequest) (*types.TextResponse, error) {
		turns++
		if turns == 1 {
			return &types.TextResponse{ToolCalls: []types.ToolCall{{ID: "id", Name: "tool"}}}, nil
		}
		if len(req.Messages) != 2 {
			t.Errorf("messages = %d, want assistant and tool result", len(req.Messages))
			return &types.TextResponse{}, nil
		}
		message, ok := req.Messages[1].(*types.ToolResultMessage)
		if !ok {
			t.Errorf("tool message has type %T", req.Messages[1])
			return &types.TextResponse{}, nil
		}
		want, marshalErr := json.Marshal(map[string]int{"call": 1})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if message.Content != string(want) || message.Error != "" {
			t.Errorf("tool message = %#v, want encoded first output", message)
		}
		return &types.TextResponse{}, nil
	}, 2)
	if err != nil || turns != 2 || calls.Load() != 1 {
		t.Fatalf("loop err=%v turns=%d encodings=%d", err, turns, calls.Load())
	}

	result := executor.Execute(context.Background(), types.ToolCall{ID: "standalone", Name: "tool"})
	if result.Error != "" {
		t.Fatalf("standalone result: %#v", result)
	}
	if _, ok := result.Result.(countingToolOutput); !ok {
		t.Fatalf("standalone result type = %T", result.Result)
	}
	message := executor.BuildToolResultMessages([]types.ToolResult{result})[0]
	want, marshalErr := json.Marshal(map[string]int{"call": 3})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if message.Content != string(want) || calls.Load() != 3 {
		t.Fatalf("standalone content=%q encodings=%d", message.Content, calls.Load())
	}
}

func TestAutomaticToolLoopPreservesOutputRejection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		output     any
		limit      int
		wantReason string
	}{
		{name: "size", output: "abc", limit: 4, wantReason: "exceeds limit"},
		{name: "serialization", output: make(chan int), limit: -1, wantReason: "failed to serialize"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			config := DefaultToolSafetyConfig()
			config.MaxToolOutputSize = tc.limit
			executor := remediationToolExecutor(t, config, map[string]types.ToolHandler{
				"tool": func(context.Context, map[string]any) (any, error) { return tc.output, nil },
			})
			var turns int
			_, err := executor.executeWithTools(context.Background(), types.TextRequest{}, func(_ context.Context, req types.TextRequest) (*types.TextResponse, error) {
				turns++
				if turns == 1 {
					return &types.TextResponse{ToolCalls: []types.ToolCall{{ID: "id", Name: "tool"}}}, nil
				}
				message, ok := req.Messages[len(req.Messages)-1].(*types.ToolResultMessage)
				if !ok || !strings.Contains(message.Error, tc.wantReason) {
					t.Errorf("rejected output message = %#v", req.Messages[len(req.Messages)-1])
				}
				return &types.TextResponse{}, nil
			}, 2)
			if err != nil || turns != 2 {
				t.Fatalf("loop turns=%d err=%v", turns, err)
			}
		})
	}
}

func TestAutomaticToolLoopRetainsHandlerErrors(t *testing.T) {
	t.Parallel()
	executor := remediationToolExecutor(t, DefaultToolSafetyConfig(), map[string]types.ToolHandler{
		"tool": func(context.Context, map[string]any) (any, error) { return nil, errors.New("handler failed") },
	})
	results, encoded := executor.executeAll(context.Background(), []types.ToolCall{{ID: "id", Name: "tool"}}, true)
	if len(results) != 1 || len(encoded) != 1 || encoded[0] != nil || !strings.Contains(results[0].Error, "handler failed") {
		t.Fatalf("results=%#v encoded=%#v", results, encoded)
	}
}
