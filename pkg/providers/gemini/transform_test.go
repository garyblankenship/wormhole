package gemini

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestToolResultMessageError(t *testing.T) {
	t.Parallel()

	g := &Gemini{}

	t.Run("error set => response.error present, result absent", func(t *testing.T) {
		t.Parallel()

		msg := &types.ToolResultMessage{
			ToolCallID:   "gemini-call-0-get_weather",
			FunctionName: "get_weather",
			Content:      "",
			Error:        "timeout",
		}

		parts, err := g.transformMessageToParts(msg, "gemini-2.5-pro")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(parts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(parts))
		}

		fnResp, ok := parts[0]["functionResponse"].(map[string]any)
		if !ok {
			t.Fatal("missing functionResponse")
		}
		response, ok := fnResp["response"].(map[string]any)
		if !ok {
			t.Fatal("missing response")
		}

		// Error path: response.error present, response.result absent
		errMap, ok := response["error"].(map[string]any)
		if !ok {
			t.Fatal("expected response.error to be present")
		}
		if errMap["message"] != "timeout" {
			t.Fatalf("expected error message 'timeout', got %q", errMap["message"])
		}
		if _, exists := response["result"]; exists {
			t.Fatal("response.result must be absent when error is set")
		}
	})

	t.Run("error empty => response.result present, error absent", func(t *testing.T) {
		t.Parallel()

		msg := &types.ToolResultMessage{
			ToolCallID:   "gemini-call-0-get_weather",
			FunctionName: "get_weather",
			Content:      `{"temp":72}`,
			Error:        "",
		}

		parts, err := g.transformMessageToParts(msg, "gemini-2.5-pro")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(parts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(parts))
		}

		fnResp, ok := parts[0]["functionResponse"].(map[string]any)
		if !ok {
			t.Fatal("missing functionResponse")
		}
		response, ok := fnResp["response"].(map[string]any)
		if !ok {
			t.Fatal("missing response")
		}

		// Success path: response.result present, response.error absent
		result, ok := response["result"]
		if !ok {
			t.Fatal("expected response.result to be present")
		}
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("expected result to be a map (parsed JSON), got %T", result)
		}
		if resultMap["temp"] != float64(72) {
			t.Fatalf("expected temp=72, got %v", resultMap["temp"])
		}
		if _, exists := response["error"]; exists {
			t.Fatal("response.error must be absent when error is empty")
		}
	})
}
