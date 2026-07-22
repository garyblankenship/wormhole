package stream

import (
	"bufio"
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
	scanner *bufio.Scanner
	event   *SSEEvent
	err     error
}

// SSEEvent represents a server-sent event
type SSEEvent struct {
	Event string
	Data  string
	ID    string
}

// NewSSEScanner creates a new SSE scanner
func NewSSEScanner(r io.Reader) *SSEScanner {
	scanner := bufio.NewScanner(r)
	// Raise the per-token cap above the default 64 KB so large SSE frames
	// (e.g. Gemini functionCall args) are not truncated with ErrTooLong.
	// Scanner includes line delimiters when deciding whether its buffer is large
	// enough. Allow CRLF beyond the policy limit, then enforce the content limit
	// explicitly below.
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSEBufferBytes+2)
	return &SSEScanner{
		scanner: scanner,
	}
}

// Scan reads the next SSE event
func (s *SSEScanner) Scan() bool {
	if s.err != nil {
		return false
	}

	event := &SSEEvent{}
	hasDataOrEvent := false

	for s.scanner.Scan() {
		physicalLine := strings.TrimSuffix(s.scanner.Text(), "\r")
		if len(physicalLine) > maxSSEBufferBytes {
			s.err = errSSEFrameTooLarge
			return false
		}
		// Strip only \r (for CRLF lines); leading spaces/tabs are significant
		// for field-name trimming inside parseSSEField, trailing are preserved.
		raw := strings.TrimRight(s.scanner.Text(), "\r")
		// Trim leading spaces/tabs only for empty-line and comment detection.
		trimmed := strings.TrimLeft(raw, " \t")

		// Empty line signals end of event
		if trimmed == "" {
			// An event is valid if it has data or event fields (even if empty)
			// This allows empty data/event fields but excludes ID-only events
			if hasDataOrEvent {
				s.event = event
				return true
			}
			continue
		}

		// Skip comments
		if strings.HasPrefix(trimmed, ":") {
			continue
		}

		// event/data fields make the event valid; id alone does not.
		if colonIndex := strings.Index(raw, ":"); colonIndex != -1 {
			if field := strings.Trim(raw[:colonIndex], " \t"); field == sseFieldEvent || field == sseFieldData {
				hasDataOrEvent = true
			}
		}

		// Parse and apply the field via the shared helper (single source of truth).
		if err := parseSSEField(raw, event); err != nil {
			s.err = err
			return false
		}
	}
	scanErr := s.scanner.Err()
	if scanErr != nil && strings.Contains(scanErr.Error(), "token too long") {
		s.err = errSSEFrameTooLarge
		return false
	}

	// Check for final event without trailing newline
	if hasDataOrEvent {
		s.event = event
		return true
	}

	s.err = scanErr
	return false
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
