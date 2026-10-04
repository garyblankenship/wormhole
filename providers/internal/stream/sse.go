package stream

import (
	"errors"
	"io"
	"strings"
)

// SSE field names
const (
	sseFieldEvent = "event"
	sseFieldData  = "data"
	sseFieldID    = "id"
)

// maxSSEBufferBytes caps a single SSE line/token at 10 MB. bufio.Scanner's
// default 64 KB (bufio.MaxScanTokenSize) is too small for large Gemini data:
// frames (big text or functionCall args), which would fail Scan with
// bufio.ErrTooLong and silently truncate the stream.
const maxSSEBufferBytes = 10 << 20

var errSSEFrameTooLarge = errors.New("SSE frame exceeds 10 MiB limit")

// SSEScanner provides a simple interface for reading Server-Sent Events
type SSEScanner struct {
	parser *SSEParser
	event  *SSEEvent
	err    error
}

// SSEEvent represents a server-sent event
type SSEEvent struct {
	Event string
	Data  string
	ID    string
}

// NewSSEScanner creates a new SSE scanner
func NewSSEScanner(r io.Reader) *SSEScanner {
	return &SSEScanner{parser: NewSSEParser(r)}
}

// Scan reads the next SSE event through the shared framing engine.
func (s *SSEScanner) Scan() bool {
	if s.err != nil {
		return false
	}
	event, err := s.parser.parseEvent(true)
	if err != nil {
		if err != io.EOF {
			s.err = err
		}
		return false
	}
	s.event = event
	return true
}

// Event returns the current event
func (s *SSEScanner) Event() *SSEEvent {
	return s.event
}

// Err returns any scanning error
func (s *SSEScanner) Err() error {
	return s.err
}

// parseSSEField parses a single SSE field line ("field: value") and applies
// the parsed field to event. It is the single source of truth for SSE field
// semantics shared by SSEScanner and SSEParser.
//
// Per the SSE spec, exactly one leading space is stripped from the value
// (strings.TrimPrefix(value, " ")); trailing whitespace is preserved.
// The field name is leniently trimmed of surrounding spaces and tabs.
// Lines without a colon are ignored (no field applied).
func parseSSEField(line string, event *SSEEvent) error {
	colonIndex := strings.Index(line, ":")
	if colonIndex == -1 {
		return nil
	}
	field := strings.Trim(line[:colonIndex], " \t")
	value := strings.TrimPrefix(line[colonIndex+1:], " ")

	switch field {
	case sseFieldEvent:
		event.Event = value
	case sseFieldData:
		newDataLen := len(event.Data) + len(value)
		if event.Data != "" {
			newDataLen++
		}
		if newDataLen > maxSSEBufferBytes {
			return errSSEFrameTooLarge
		}
		if event.Data != "" {
			event.Data += "\n"
		}
		event.Data += value
	case sseFieldID:
		event.ID = value
	}
	return nil
}
