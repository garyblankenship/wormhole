package wormhole

import (
	"fmt"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

// Keep the total payload fixed so fragment count measures assembly overhead,
// rather than increasing the amount of JSON being parsed and normalized.
func BenchmarkTextResponseCollectorRawArguments(b *testing.B) {
	const size = 16 * 1024
	const prefix, suffix = `{"value":"`, `"}`
	value := strings.Repeat("x", size-len(prefix)-len(suffix))
	raw := prefix + value + suffix
	for _, fragments := range []int{1, 32, 512} {
		b.Run(fmt.Sprintf("bytes=%d/fragments=%d", size, fragments), func(b *testing.B) {
			chunks := make([]types.TextChunk, fragments)
			for i := range chunks {
				call := types.ToolCall{Index: 7, Function: &types.ToolCallFunction{
					Arguments: raw[i*len(raw)/fragments : (i+1)*len(raw)/fragments],
				}}
				if i == 0 {
					call.ID = "call"
					call.Function.Name = "lookup"
				}
				chunks[i].Delta = &types.ChunkDelta{ToolCalls: []types.ToolCall{call}}
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			var result types.TextResponse
			for i := 0; i < b.N; i++ {
				var collector textResponseCollector
				for _, chunk := range chunks {
					collector.add(chunk)
				}
				collector.finish()
				result = collector.response
			}
			b.StopTimer()
			if len(result.ToolCalls) != 1 {
				b.Fatalf("got %d calls, want 1", len(result.ToolCalls))
			}
			call := result.ToolCalls[0]
			if call.ID != "call" || call.Name != "lookup" || call.ArgsInvalid ||
				call.Function == nil || call.Function.Arguments != raw || call.Arguments["value"] != value {
				b.Fatal("collected arguments differ from the fixed input")
			}
		})
	}
}
