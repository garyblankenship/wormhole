package anthropic

import (
	"testing"
)

// message_start carries Anthropic's input + cache tokens; only output_tokens
// arrives later on message_delta. The stream parser must NOT drop the
// message_start usage. Regression guard.
func TestParseStreamChunkMessageStartCapturesUsage(t *testing.T) {
	t.Parallel()
	p := &Provider{}

	data := []byte(`{"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"output_tokens":0,"cache_read_input_tokens":30,"cache_creation_input_tokens":20}}}`)

	chunk, err := p.parseStreamChunk(data)
	if err != nil {
		t.Fatalf("parseStreamChunk: %v", err)
	}
	if chunk == nil || chunk.Usage == nil {
		t.Fatalf("expected non-nil chunk.Usage, got chunk=%v", chunk)
	}
	if chunk.Usage.PromptTokens != 100 {
		t.Fatalf("PromptTokens = %d, want 100", chunk.Usage.PromptTokens)
	}
	if chunk.Usage.CacheReadTokens != 30 {
		t.Fatalf("CacheReadTokens = %d, want 30", chunk.Usage.CacheReadTokens)
	}
	if chunk.Usage.CacheWriteTokens != 20 {
		t.Fatalf("CacheWriteTokens = %d, want 20", chunk.Usage.CacheWriteTokens)
	}
}
