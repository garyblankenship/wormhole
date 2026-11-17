package server

import (
	"net/http"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

// These benchmarks deliberately use pre-change entrypoints and fields so the
// identical file can be overlaid onto the baseline without compatibility shims.
type chatBenchmarkSink struct{ bytes int }

func (*chatBenchmarkSink) Header() http.Header { return nil }
func (*chatBenchmarkSink) WriteHeader(int)     {}
func (w *chatBenchmarkSink) Write(data []byte) (int, error) {
	w.bytes += len(data)
	return len(data), nil
}
func (*chatBenchmarkSink) Flush() {}

func BenchmarkChatStreamFrame(b *testing.B) {
	state := newChatStreamState("fixture", nil)
	p := &proxy{}
	writer := &chatBenchmarkSink{}
	response := ChatCompletionResponse{ID: "fixture", Object: "chat.completion.chunk", Created: 123, Model: "fixture", Choices: []ChatChoice{{Index: 0, Delta: &ChatDelta{Content: "ordinary text chunk with unicode 雪"}}}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !state.writeResponse(p, writer, writer, response) {
			b.Fatal("frame write failed")
		}
	}
	b.ReportMetric(float64(writer.bytes)/float64(b.N), "wire-B/op")
}

func BenchmarkChatStreamReferences(b *testing.B) {
	// Every operation drains the same 256 chunks of eight references per field.
	// Input construction is outside timing; only stream state and accumulation
	// vary with include selection. No terminal snapshot masks retained history.
	chunk := types.TextChunk{}
	for i := 0; i < 8; i++ {
		chunk.Sources = append(chunk.Sources, types.Source{ID: "fixture", URL: "https://example.com/source"})
		chunk.Citations = append(chunk.Citations, types.Citation{Raw: map[string]any{"type": "url_citation", "url_citation": map[string]any{"url": "https://example.com/source"}}})
		chunk.Warnings = append(chunk.Warnings, types.ResponseWarning{})
	}
	chunks := make([]types.TextChunk, 256)
	for i := range chunks {
		chunks[i] = chunk
	}
	for _, tc := range []struct {
		name    string
		include []string
	}{
		{name: "none"},
		{name: "one", include: []string{"citations"}},
		{name: "all", include: []string{"sources", "citations", "warnings"}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var extension *WormholeRequest
			if tc.include != nil {
				extension = &WormholeRequest{Include: tc.include}
			}
			retained := 0
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state := newChatStreamState("fixture", extension)
				for _, next := range chunks {
					state.absorb(next)
				}
				retained = len(state.sources) + len(state.citations) + len(state.warnings)
			}
			b.ReportMetric(float64(retained), "retained-refs/op")
			b.ReportMetric(float64(len(chunks)), "chunks/op")
		})
	}
}
