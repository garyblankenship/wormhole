package stream

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"github.com/garyblankenship/wormhole/v2/types"
)

// StreamProcessor processes streaming responses from providers
type StreamProcessor struct {
	parser      *SSEParser
	transformer func([]byte) (*types.TextChunk, error)
}

// NewStreamProcessor creates a new stream processor
func NewStreamProcessor(r io.Reader, transformer func([]byte) (*types.TextChunk, error)) *StreamProcessor {
	return &StreamProcessor{
		parser:      NewSSEParser(r),
		transformer: transformer,
	}
}

// Process processes the stream and sends chunks to the channel.
// Every send is guarded by ctx.Done() so the goroutine exits if the
// consumer stops reading (otherwise the send blocks forever, holding
// the upstream body open).
func (p *StreamProcessor) Process(ctx context.Context, chunks chan<- types.TextChunk) {
	defer close(chunks)

	var finished bool

	for {
		event, err := p.parser.Parse()
		if err != nil {
			if err == io.EOF {
				if !finished {
					select {
					case chunks <- types.TextChunk{Error: fmt.Errorf("stream ended prematurely: no terminal finish event received")}:
					case <-ctx.Done():
					}
				}
			} else {
				select {
				case chunks <- types.TextChunk{Error: err}:
				case <-ctx.Done():
				}
			}
			return
		}

		// Skip non-data events
		if event.Data == "" {
			continue
		}

		// Handle [DONE] marker
		if event.Data == streamDoneMarker {
			return
		}

		// Transform the data
		chunk, err := p.transformer([]byte(event.Data))
		if err != nil {
			select {
			case chunks <- types.TextChunk{Error: fmt.Errorf("failed to parse chunk: %w", err)}:
			case <-ctx.Done():
			}
			return
		}

		if chunk != nil {
			if chunk.IsDone() {
				finished = true
			}
			select {
			case chunks <- *chunk:
			case <-ctx.Done():
				return
			}
		}
	}
}

// ProcessSSE creates and processes an SSE stream in a goroutine, returning the channel.
// This is a convenience function that combines channel creation, goroutine launch, and processing.
// ctx cancellation unblocks the producer goroutine's sends and lets the body close.
func ProcessSSE(
	ctx context.Context,
	body io.ReadCloser,
	transformer func([]byte) (*types.TextChunk, error),
	bufferSize int,
) <-chan types.TextChunk {
	chunks := make(chan types.TextChunk, bufferSize)
	go func() {
		defer func() {
			_ = body.Close()
		}()
		processor := NewStreamProcessor(body, transformer)
		processor.Process(ctx, chunks)
	}()
	return chunks
}

// ProcessNDJSON processes an NDJSON (newline-delimited JSON) stream.
// Each line is a complete JSON object with no SSE framing (used by Ollama).
// ctx cancellation unblocks the producer goroutine's sends and lets the body close.
func ProcessNDJSON(
	ctx context.Context,
	body io.ReadCloser,
	transformer func([]byte) (*types.TextChunk, error),
	bufferSize int,
) <-chan types.TextChunk {
	chunks := make(chan types.TextChunk, bufferSize)
	go func() {
		defer func() {
			close(chunks)
			_ = body.Close()
		}()
		scanner := bufio.NewScanner(body)
		// Ollama can return large final chunks with usage data.
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			chunk, err := transformer(line)
			if err != nil {
				select {
				case chunks <- types.TextChunk{Error: fmt.Errorf("failed to parse NDJSON chunk: %w", err)}:
				case <-ctx.Done():
					return
				}
				return
			}
			if chunk == nil {
				continue
			}
			select {
			case chunks <- *chunk:
			case <-ctx.Done():
				return
			}
			if chunk.IsDone() {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case chunks <- types.TextChunk{Error: fmt.Errorf("NDJSON scan error: %w", err)}:
			case <-ctx.Done():
			}
			return
		}
		select {
		case chunks <- types.TextChunk{Error: fmt.Errorf("NDJSON stream ended before terminal chunk")}:
		case <-ctx.Done():
		}
	}()
	return chunks
}
