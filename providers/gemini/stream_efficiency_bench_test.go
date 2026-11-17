package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

func writeEfficiencyEvent(tb testing.TB, wire *strings.Builder, frame geminiTextResponse) {
	tb.Helper()
	encoded, err := json.Marshal(frame)
	if err != nil {
		tb.Fatal(err)
	}
	wire.WriteString("data: ")
	wire.Write(encoded)
	wire.WriteString("\n\n")
}

func efficiencyGrounding(start, end int) map[string]any {
	return map[string]any{
		"groundingChunks": []any{map[string]any{"web": map[string]any{"uri": "https://example.com/source"}}},
		"groundingSupports": []any{map[string]any{
			"segment":               map[string]any{"startIndex": start, "endIndex": end},
			"groundingChunkIndices": []any{0},
		}},
	}
}

// Each event adds 256 bytes; input encoding is outside the timed loop. Both
// variants drain the real stream owner, including terminal reference snapshots.
func BenchmarkHandleStreamEfficiency(b *testing.B) {
	for _, grounding := range []bool{false, true} {
		kind := "text"
		if grounding {
			kind = "growing_grounding"
		}
		for _, frames := range []int{64, 256, 1024} {
			b.Run(fmt.Sprintf("%s/frames_%d", kind, frames), func(b *testing.B) {
				var wire strings.Builder
				text := strings.Repeat("é界x", 42) + "abcd" // 256 UTF-8 bytes.
				for frame := 0; frame < frames; frame++ {
					c := candidate{Content: content{Parts: []part{{Text: text}}}}
					if grounding {
						c.GroundingMetadata = efficiencyGrounding(0, (frame+1)*len(text))
					}
					writeEfficiencyEvent(b, &wire, geminiTextResponse{Candidates: []candidate{c}})
				}
				writeEfficiencyEvent(b, &wire, geminiTextResponse{Candidates: []candidate{{FinishReason: "STOP"}}})
				wire.WriteString("data: [DONE]\n\n")
				input := wire.String()
				provider := &Gemini{}
				ctx := context.Background()
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					textBytes, terminalCount, citations := 0, 0, 0
					for chunk := range provider.handleStream(ctx, io.NopCloser(strings.NewReader(input))) {
						if chunk.Error != nil {
							b.Fatal(chunk.Error)
						}
						textBytes += len(chunk.Text)
						if chunk.FinishReason != nil {
							terminalCount++
							citations = len(chunk.Citations)
							if len(chunk.Warnings) != 0 {
								b.Fatalf("warnings=%#v", chunk.Warnings)
							}
						}
					}
					wantCitations := 0
					if grounding {
						wantCitations = frames
					}
					if textBytes != frames*len(text) || terminalCount != 1 || citations != wantCitations {
						b.Fatalf("textBytes=%d terminalCount=%d citations=%d", textBytes, terminalCount, citations)
					}
				}
			})
		}
	}
}
