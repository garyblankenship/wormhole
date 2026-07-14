package stream

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// Stream sentinel value
const streamDoneMarker = "[DONE]"

// lineBufferPool pools byte slices for line reading to reduce allocations.
// Stores *[]byte so sync.Pool.Put receives a pointer type (SA6002).
var lineBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 1024)
		return &buf
	},
}

// SSEParser parses Server-Sent Events streams
type SSEParser struct {
	reader *bufio.Reader
}

// sseReaderBufferSize sizes the SSE line buffer. Large single frames are routine
// (OpenAI Responses `response.completed` objects, big tool-call argument deltas,
// gateway-batched deltas), so the default 4KB bufio buffer would otherwise force
// the ErrBufferFull path on real traffic.
const sseReaderBufferSize = 1 << 20 // 1 MiB

// NewSSEParser creates a new SSE parser
func NewSSEParser(r io.Reader) *SSEParser {
	return &SSEParser{
		reader: bufio.NewReaderSize(r, sseReaderBufferSize),
	}
}

// Remove duplicate SSEEvent type - using the one from sse.go

// Parse reads and parses the next SSE event
func (p *SSEParser) Parse() (*SSEEvent, error) {
	event := &SSEEvent{}

	for {
		line, eof, err := p.readLine()
		if err != nil {
			return nil, err
		}

		// Check if we should return the event (EOF or empty line with data)
		shouldReturn, returnErr := p.shouldReturnWithError(line, event, eof)
		if returnErr != nil {
			p.returnToPool(line)
			return nil, returnErr
		}
		if shouldReturn {
			p.returnToPool(line)
			return event, nil
		}

		// Skip empty lines and comments, but check for EOF first
		if p.shouldSkip(line) {
			// If we hit EOF while skipping empty lines, return EOF if no event data
			if eof && !p.hasEventData(event) {
				p.returnToPool(line)
				return nil, io.EOF
			}
			p.returnToPool(line)
			continue
		}

		// Parse and apply field to event
		if err := p.parseField(line, event); err != nil {
			p.returnToPool(line)
			continue // Invalid field format, skip
		}
		p.returnToPool(line)

		// Return event if we reached EOF after processing
		if eof {
			return event, nil
		}
	}
}

// readLine reads next line and handles EOF
func (p *SSEParser) readLine() ([]byte, bool, error) {
	// Use ReadSlice to avoid allocation, then copy to pooled buffer
	slice, err := p.reader.ReadSlice('\n')

	// Helper to trim trailing newline characters
	trimNewline := func(s []byte) []byte {
		if len(s) > 0 && s[len(s)-1] == '\n' {
			s = s[:len(s)-1]
			if len(s) > 0 && s[len(s)-1] == '\r' {
				s = s[:len(s)-1]
			}
		}
		return s
	}

	// Check if we need to handle EOF
	if err == io.EOF {
		if len(slice) == 0 {
			return nil, true, io.EOF
		}
		// Have data before EOF, process it
		trimmed := trimNewline(slice)
		line := p.copyToPooledBuffer(trimmed)
		return line, true, nil
	}

	// Handle other errors (buffer too small)
	if err != nil && err != bufio.ErrBufferFull {
		return nil, false, err
	}

	// Line exceeds the (already large) bufio buffer. slice holds the consumed
	// prefix and is valid only until the next read, so copy it, then read the
	// remainder and concatenate. The previous code discarded the prefix, silently
	// dropping the first buffer-size bytes — and thus the whole frame's `data:` field.
	if err == bufio.ErrBufferFull {
		prefix := append([]byte(nil), slice...)
		rest, err2 := p.reader.ReadString('\n')
		if err2 != nil && err2 != io.EOF {
			return nil, false, err2
		}
		full := append(prefix, rest...)
		trimmed := trimNewline(full)
		line := p.copyToPooledBuffer(trimmed)
		return line, err2 == io.EOF, nil
	}

	// Normal case: successful read
	trimmed := trimNewline(slice)
	line := p.copyToPooledBuffer(trimmed)
	return line, false, nil
}

// copyToPooledBuffer copies a byte slice to a pooled buffer
// Caller must return the buffer to pool after use
func (p *SSEParser) copyToPooledBuffer(slice []byte) []byte {
	bufPtr := lineBufferPool.Get().(*[]byte)
	buf := (*bufPtr)[:0] // reset length
	buf = append(buf, slice...)
	return buf
}

// returnToPool returns a buffer to the pool
// This should be called by Parse after processing a line
func (p *SSEParser) returnToPool(buf []byte) {
	buf = buf[:0]
	lineBufferPool.Put(&buf)
}

// shouldReturn checks if event is complete and should be returned
// Returns (shouldReturn bool, returnError error)
func (p *SSEParser) shouldReturnWithError(line []byte, event *SSEEvent, isEOF bool) (bool, error) {
	// Empty line signals end of event
	if len(line) == 0 {
		if p.hasEventData(event) {
			return true, nil
		}
		// At EOF with no data, return EOF error
		if isEOF {
			if event.Data != "" || event.Event != "" || event.ID != "" {
				return true, nil
			}
			return false, io.EOF
		}
	}
	return false, nil
}

// shouldSkip checks if line should be skipped (comments)
func (p *SSEParser) shouldSkip(line []byte) bool {
	return len(line) == 0 || (len(line) > 0 && line[0] == ':')
}

// hasEventData checks if event has meaningful data
func (p *SSEParser) hasEventData(event *SSEEvent) bool {
	return event.Data != "" || event.Event != ""
}

// parseField parses a field line and updates the event via the shared
// parseSSEField helper (single source of truth for SSE field semantics).
func (p *SSEParser) parseField(line []byte, event *SSEEvent) error {
	if !bytes.Contains(line, []byte(":")) {
		return fmt.Errorf("invalid field format")
	}

	parseSSEField(string(line), event)
	return nil
}

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
