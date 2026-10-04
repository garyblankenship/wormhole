package stream

import (
	"bufio"
	"io"
	"strings"
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
	reader     *bufio.Reader
	pendingErr error
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

// Parse reads and parses the next SSE event.
func (p *SSEParser) Parse() (*SSEEvent, error) {
	return p.parseEvent(false)
}

// parseEvent is the framing engine for both public facades. The scanner policy
// retains its empty-field events, whitespace boundaries and deferred read errors.
func (p *SSEParser) parseEvent(scanner bool) (*SSEEvent, error) {
	event := &SSEEvent{}
	hasFields := false
	for {
		buf, eof, err := p.readLine(scanner)
		if err != nil {
			if scanner && hasFields && err != errSSEFrameTooLarge {
				p.pendingErr = err
				return event, nil
			}
			return nil, err
		}
		line := string(*buf)
		p.returnToPool(buf)
		boundary := line
		if scanner {
			line = strings.TrimRight(line, "\r")
			boundary = strings.TrimLeft(line, " \t")
		}
		valid := event.Data != "" || event.Event != ""
		if scanner {
			valid = hasFields
		}
		if boundary == "" {
			if valid {
				return event, nil
			}
			if eof {
				return nil, io.EOF
			}
			continue
		}
		if strings.HasPrefix(boundary, ":") {
			if eof {
				if valid {
					return event, nil
				}
				return nil, io.EOF
			}
			continue
		}
		if colon := strings.IndexByte(line, ':'); colon >= 0 {
			field := strings.Trim(line[:colon], " \t")
			if field == sseFieldData || field == sseFieldEvent {
				hasFields = true
			}
			if err := parseSSEField(line, event); err != nil {
				return nil, err
			}
			if eof {
				if scanner && !hasFields {
					return nil, io.EOF
				}
				return event, nil
			}
		} else if eof {
			if valid {
				return event, nil
			}
			return nil, io.EOF
		}
	}
}

// readLine retains the original pool pointer through every success/error path.
// Scanner exposes a partial final line before its reader error; Parser preserves
// its historical immediate reader-error behavior.
func (p *SSEParser) readLine(scanner bool) (*[]byte, bool, error) {
	if p.pendingErr != nil {
		err := p.pendingErr
		p.pendingErr = nil
		return nil, false, err
	}
	buf := lineBufferPool.Get().(*[]byte)
	*buf = (*buf)[:0]
	for {
		fragment, err := p.reader.ReadSlice('\n')
		if len(*buf)+len(fragment) > maxSSEBufferBytes+2 {
			p.returnToPool(buf)
			return nil, false, errSSEFrameTooLarge
		}
		*buf = append(*buf, fragment...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil && err != io.EOF {
			if !scanner || len(*buf) == 0 {
				p.returnToPool(buf)
				return nil, false, err
			}
			p.pendingErr = err
		}
		if err == io.EOF && len(*buf) == 0 {
			p.returnToPool(buf)
			return nil, true, io.EOF
		}
		if n := len(*buf); n > 0 && (*buf)[n-1] == '\n' {
			*buf = (*buf)[:n-1]
		}
		if n := len(*buf); n > 0 && (*buf)[n-1] == '\r' {
			*buf = (*buf)[:n-1]
		}
		if len(*buf) > maxSSEBufferBytes {
			p.returnToPool(buf)
			return nil, false, errSSEFrameTooLarge
		}
		return buf, err != nil, nil
	}
}

func (p *SSEParser) returnToPool(buf *[]byte) {
	*buf = (*buf)[:0]
	lineBufferPool.Put(buf)
}
