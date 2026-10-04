package stream

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// C4-I02: facades share framing while retaining their established policies.
func TestRemediationC4I02FramingFacades(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, input string
		want        []SSEEvent
	}{
		{"CRLF multiline final", ": comment\r\nevent: message\r\ndata: first\r\ndata: second\r\nid: 1\r\n\r\ndata: final", []SSEEvent{{Event: "message", Data: "first\nsecond", ID: "1"}, {Data: "final"}}},
		{"blank and comments", "\n: comment\n\ndata: one\n\n\n: end", []SSEEvent{{Data: "one"}}},
		{"trailing whitespace", "data:  kept \t\n\n", []SSEEvent{{Data: " kept \t"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parser := NewSSEParser(strings.NewReader(tc.input))
			scanner := NewSSEScanner(strings.NewReader(tc.input))
			for _, want := range tc.want {
				got, err := parser.Parse()
				if err != nil || *got != want {
					t.Fatalf("parser=%#v err=%v want=%#v", got, err, want)
				}
				if !scanner.Scan() || *scanner.Event() != want {
					t.Fatalf("scanner=%#v err=%v want=%#v", scanner.Event(), scanner.Err(), want)
				}
			}
			if _, err := parser.Parse(); err != io.EOF {
				t.Fatalf("parser EOF=%v", err)
			}
			if scanner.Scan() || scanner.Err() != nil {
				t.Fatalf("scanner EOF=%v", scanner.Err())
			}
		})
	}
	// Empty fields and whitespace boundaries are historical scanner extensions.
	scanner := NewSSEScanner(strings.NewReader("data:\n \t\n"))
	if !scanner.Scan() || scanner.Event().Data != "" {
		t.Fatal("empty scanner event lost")
	}
	if _, err := NewSSEParser(strings.NewReader("data:\n\n")).Parse(); err != io.EOF {
		t.Fatalf("parser empty event=%v", err)
	}
}

func TestRemediationC4I02ReaderFailuresAndLimits(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("reader failure")
	for _, scannerMode := range []bool{false, true} {
		parser := NewSSEParser(&partialErrorReader{data: []byte("data: partial"), err: wantErr})
		event, err := parser.parseEvent(scannerMode)
		if scannerMode {
			if err != nil || event.Data != "partial" {
				t.Fatalf("scanner partial=%#v err=%v", event, err)
			}
			_, err = parser.parseEvent(scannerMode)
		}
		if !errors.Is(err, wantErr) {
			t.Fatalf("reader error=%v", err)
		}
	}
	for _, input := range []string{
		"data: " + strings.Repeat("x", maxSSEBufferBytes) + "\n\n",
		"data: " + strings.Repeat("x", maxSSEBufferBytes/2) + "\ndata: " + strings.Repeat("x", maxSSEBufferBytes/2) + "\n\n",
	} {
		if _, err := NewSSEParser(strings.NewReader(input)).Parse(); !errors.Is(err, errSSEFrameTooLarge) {
			t.Fatalf("parser limit=%v", err)
		}
		scanner := NewSSEScanner(strings.NewReader(input))
		if scanner.Scan() || !errors.Is(scanner.Err(), errSSEFrameTooLarge) {
			t.Fatalf("scanner limit=%v", scanner.Err())
		}
	}
}

// C4-10: concurrent line ownership cannot corrupt another parser's event.
func TestRemediationC410PoolConcurrentParsers(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	for worker := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := strings.Repeat(string(rune('a'+worker)), 100000)
			for range 10 {
				parser := NewSSEParser(strings.NewReader("data: " + data + "\n\n"))
				event, err := parser.Parse()
				if err != nil || event.Data != data {
					t.Errorf("worker %d event corrupted: %v", worker, err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

type remediationSSEBody struct {
	io.Reader
	closes atomic.Int32
}

func (b *remediationSSEBody) Close() error { b.closes.Add(1); return nil }

func TestRemediationC4R02TerminalErrorEvents(t *testing.T) {
	t.Parallel()
	preserved := errors.New("typed provider failure")
	for _, tc := range []struct {
		name, payload string
		providerError bool
	}{
		{"empty", "", false}, {"opaque", "opaque", false}, {"transformed", "typed", true}, {"parse failure", "invalid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := "data: partial\n\nevent: error\ndata: " + tc.payload + "\n\ndata: forbidden\n\n"
			body := &remediationSSEBody{Reader: strings.NewReader(input)}
			calls := 0
			transformer := func(data []byte) (*types.TextChunk, error) {
				calls++
				if string(data) == "typed" {
					return &types.TextChunk{Error: preserved}, nil
				}
				if string(data) == "invalid" {
					return nil, preserved
				}
				return &types.TextChunk{Text: string(data)}, nil
			}
			var chunks []types.TextChunk
			for chunk := range ProcessSSE(context.Background(), body, transformer, 1) {
				chunks = append(chunks, chunk)
			}
			if len(chunks) != 2 || chunks[0].Text != "partial" || chunks[1].Error == nil {
				t.Fatalf("chunks=%#v", chunks)
			}
			if tc.providerError && chunks[1].Error != preserved {
				t.Fatalf("provider error lost: %v", chunks[1].Error)
			}
			if calls > 2 || body.closes.Load() != 1 {
				t.Fatalf("transform calls=%d closes=%d", calls, body.closes.Load())
			}
		})
	}
}

func TestRemediationC4R02TransformedErrorWithoutHeader(t *testing.T) {
	t.Parallel()
	body := &remediationSSEBody{Reader: strings.NewReader("data: fail\n\ndata: forbidden\n\n")}
	calls := 0
	for chunk := range ProcessSSE(context.Background(), body, func([]byte) (*types.TextChunk, error) {
		calls++
		return &types.TextChunk{Error: errors.New("provider")}, nil
	}, 1) {
		if chunk.Error == nil {
			t.Fatal("missing provider failure")
		}
	}
	if calls != 1 || body.closes.Load() != 1 {
		t.Fatalf("calls=%d closes=%d", calls, body.closes.Load())
	}
}

func TestRemediationC4R02CancellationClosesBlockedBodyOnce(t *testing.T) {
	t.Parallel()
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	var closes atomic.Int32
	body := &remediationBlockingBody{PipeReader: reader, closes: &closes}
	ctx, cancel := context.WithCancel(context.Background())
	chunks := ProcessSSE(ctx, body, func([]byte) (*types.TextChunk, error) { return nil, nil }, 1)
	cancel()
	done := make(chan struct{})
	go func() {
		for range chunks {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation left reader blocked")
	}
	if closes.Load() != 1 {
		t.Fatalf("closes=%d", closes.Load())
	}
}

type remediationBlockingBody struct {
	*io.PipeReader
	closes *atomic.Int32
}

func (b *remediationBlockingBody) Close() error { b.closes.Add(1); return b.PipeReader.Close() }
