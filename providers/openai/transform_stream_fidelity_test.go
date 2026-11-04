package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

// TestParseStreamChunkProviderUsageFidelity pins the streaming provider_usage
// contract on the unified transformer path: ordinary deltas carry no metadata,
// and usage-bearing chunks expose the raw usage object, including fields the
// typed structs do not model. The equivalent non-streaming Responses-path
// contract is covered by TestV330ResponsesMetadataUsagePrewarm.
func TestParseStreamChunkProviderUsageFidelity(t *testing.T) {
	t.Parallel()
	p := New(types.NewProviderConfig("key"))

	chunk, err := p.parseStreamChunk([]byte(`{"id":"chatcmpl-1","model":"gpt-test","choices":[{"index":0,"delta":{"content":"hel"},"finish_reason":null}]}`))
	require.NoError(t, err)
	require.NotNil(t, chunk)
	require.Equal(t, "hel", chunk.Text)
	require.Nil(t, chunk.Metadata, "ordinary delta chunks must not carry provider_usage metadata")

	chunk, err = p.parseStreamChunk([]byte(`{"id":"chatcmpl-1","model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"prompt_tokens_details":{"cached_tokens":2},"future_detail":true}}`))
	require.NoError(t, err)
	require.NotNil(t, chunk)
	require.NotNil(t, chunk.FinishReason)
	require.Equal(t, types.FinishReasonStop, *chunk.FinishReason)
	require.NotNil(t, chunk.Usage)
	require.Equal(t, 5, chunk.Usage.TotalTokens)
	require.Equal(t, 2, chunk.Usage.CacheReadTokens)

	raw, ok := chunk.Metadata["provider_usage"].(map[string]any)
	require.True(t, ok, "usage-bearing chunk must expose provider_usage")
	require.Equal(t, true, raw["future_detail"], "unknown usage fields must survive")
	details, ok := raw["prompt_tokens_details"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(2), details["cached_tokens"])
}

func BenchmarkParseStreamChunkDelta(b *testing.B) {
	p := New(types.NewProviderConfig("key"))
	data := []byte(`{"id":"chatcmpl-1","model":"gpt-test","choices":[{"index":0,"delta":{"content":"token fragment"},"finish_reason":null}]}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.parseStreamChunk(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseStreamChunkUsage(b *testing.B) {
	p := New(types.NewProviderConfig("key"))
	data := []byte(`{"id":"chatcmpl-1","model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"prompt_tokens_details":{"cached_tokens":2},"future_detail":true}}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.parseStreamChunk(data); err != nil {
			b.Fatal(err)
		}
	}
}
