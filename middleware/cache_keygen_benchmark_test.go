package middleware

import (
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

// cacheKeygenRequest builds a representative TextRequest for key-generation
// cost measurement: N user messages of roughly textLen bytes each, T tool
// definitions with JSON schemas, and nonempty ProviderOptions (which the
// default key generator marshals separately because the field is json:"-").
func cacheKeygenRequest(messages, tools, textLen int) types.TextRequest {
	text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", textLen/45+1)
	msgs := make([]types.Message, messages)
	for i := range msgs {
		msgs[i] = types.NewUserMessage(strings.TrimSpace(text[:textLen]))
	}
	toolDefs := make([]types.Tool, tools)
	for i := range toolDefs {
		toolDefs[i] = types.Tool{
			Type:        "function",
			Name:        strings.Repeat("tool_", 1) + strings.Repeat("n", i%8),
			Description: "Benchmark tool definition with a realistic description length.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer"},
				},
				"required": []any{"query"},
			},
		}
	}
	return types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gpt-test",
			ProviderOptions: map[string]any{
				"temperature": 0.2,
				"top_p":       0.9,
				"user":        "bench",
			},
		},
		Messages: msgs,
		Tools:    toolDefs,
	}
}

// BenchmarkDefaultCacheKeyGeneratorTiers measures the default key generator
// (request JSON + separate ProviderOptions JSON + SHA-256) across request
// sizes. Key generation runs on every eligible cache lookup, hit or miss, so
// its cost scales with request size even when the provider call is skipped.
func BenchmarkDefaultCacheKeyGeneratorTiers(b *testing.B) {
	cases := []struct {
		name     string
		messages int
		tools    int
		textLen  int
	}{
		{"small_1msg", 1, 0, 160},
		{"medium_40msg_6tools", 40, 6, 400},
		{"large_400msg_16tools", 400, 16, 800},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			req := cacheKeygenRequest(tc.messages, tc.tools, tc.textLen)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := DefaultCacheKeyGenerator(req); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
