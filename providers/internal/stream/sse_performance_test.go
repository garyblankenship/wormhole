package stream

import (
	"errors"
	"io"
	"strings"
	"testing"
)

var (
	benchmarkSSEParserSink *SSEParser
	benchmarkSSEEventSink  *SSEEvent
	benchmarkSSEErrorSink  error
)

func BenchmarkSSEParserConstruction(b *testing.B) {
	reader := strings.NewReader("")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSSEParserSink = NewSSEParser(reader)
	}
}

func BenchmarkSSEParserShortEvents(b *testing.B) {
	const eventsPerIteration = 128
	input := strings.Repeat("event: message\ndata: hello\nid: 1\n\n", eventsPerIteration)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser := NewSSEParser(strings.NewReader(input))
		for event := 0; event < eventsPerIteration; event++ {
			benchmarkSSEEventSink, benchmarkSSEErrorSink = parser.Parse()
			if benchmarkSSEErrorSink != nil {
				b.Fatal(benchmarkSSEErrorSink)
			}
		}
	}
}

func BenchmarkSSEParserFragmentedLargeEvent(b *testing.B) {
	input := "data: " + strings.Repeat("x", 2<<20) + "\n\n"
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser := NewSSEParser(strings.NewReader(input))
		benchmarkSSEEventSink, benchmarkSSEErrorSink = parser.Parse()
		if benchmarkSSEErrorSink != nil {
			b.Fatal(benchmarkSSEErrorSink)
		}
	}
}

func TestSSEParserLineEndingsAndReaderErrors(t *testing.T) {
	t.Run("CRLF", func(t *testing.T) {
		parser := NewSSEParser(strings.NewReader("event: message\r\ndata: first\r\ndata: second\r\n\r\n"))
		event, err := parser.Parse()
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if event.Event != "message" || event.Data != "first\nsecond" {
			t.Fatalf("Parse() event = %#v", event)
		}
	})

	t.Run("partial reader error", func(t *testing.T) {
		wantErr := errors.New("reader failed")
		parser := NewSSEParser(&partialErrorReader{
			data: []byte("data: partial"),
			err:  wantErr,
		})
		_, err := parser.Parse()
		if !errors.Is(err, wantErr) {
			t.Fatalf("Parse() error = %v, want %v", err, wantErr)
		}
	})
}

type partialErrorReader struct {
	data []byte
	err  error
	done bool
}

func (r *partialErrorReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, r.data), r.err
}

var _ io.Reader = (*partialErrorReader)(nil)
