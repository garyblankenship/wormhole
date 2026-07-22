package stream

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
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

// sseReaderBufferSize keeps common lines in the reader without eagerly reserving
// a large frame-sized allocation for every stream. readLine assembles larger
// frames from bounded fragments up to maxSSEBufferBytes.
const sseReaderBufferSize = 64 << 10 // 64 KiB

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
			if errors.Is(err, errSSEFrameTooLarge) {
				return nil, err
			}
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
	bufPtr := lineBufferPool.Get().(*[]byte)
	line := (*bufPtr)[:0]
	release := func() {
		line = line[:0]
		*bufPtr = line
		lineBufferPool.Put(bufPtr)
	}

	for {
		fragment, err := p.reader.ReadSlice('\n')
		// Permit the maximum line content plus CRLF while ensuring an upstream
		// peer can never make this buffer grow without bound.
		if len(line)+len(fragment) > maxSSEBufferBytes+2 {
			release()
			return nil, false, errSSEFrameTooLarge
		}
		line = append(line, fragment...)

		switch err {
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			if len(line) == 0 {
				release()
				return nil, true, io.EOF
			}
		case nil:
		default:
			release()
			return nil, false, err
		}

		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if len(line) > maxSSEBufferBytes {
			release()
			return nil, false, errSSEFrameTooLarge
		}
		return line, err == io.EOF, nil
	}
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

	return parseSSEField(string(line), event)
}
