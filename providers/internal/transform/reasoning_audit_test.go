package transform

import (
	"encoding/json"
	"testing"
)

func TestAuditReasoningDeltaPreservesTextAndRefusal(t *testing.T) {
	t.Parallel()
	transformer := NewStreamingTransformer(StreamingConfig{ThinkingPath: "thinking", TextFieldPath: "text", RefusalPath: "refusal"})
	for _, combined := range []bool{false, true} {
		frame := map[string]any{"thinking": "reasoning"}
		if combined {
			frame["text"], frame["refusal"] = "answer", "refused"
		}
		raw, err := json.Marshal(frame)
		if err != nil {
			t.Fatal(err)
		}
		chunk, err := transformer.ParseChunk(raw)
		if err != nil {
			t.Fatal(err)
		}
		if chunk.Thinking == nil || chunk.Thinking.Content != "reasoning" || chunk.Delta == nil || chunk.Delta.Thinking == nil || chunk.Delta.Thinking.Content != "reasoning" {
			t.Fatalf("reasoning=%#v", chunk)
		}
		if combined && (chunk.Text != "answer" || chunk.Delta.Content != "answer" || chunk.Refusal != "refused" || chunk.Delta.Refusal != "refused") {
			t.Fatalf("combined=%#v delta=%#v", chunk, chunk.Delta)
		}
	}
}
