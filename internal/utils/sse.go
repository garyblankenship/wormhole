package utils

import (
	"bufio"
	"io"
	"strings"
)

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
	return &SSEScanner{
		scanner: bufio.NewScanner(r),
	}
}

// Scan reads the next SSE event
func (s *SSEScanner) Scan() bool {
	event := &SSEEvent{}
	hasDataOrEvent := false

	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())

		// Empty line signals end of event
		if line == "" {
			// An event is valid if it has data or event fields (even if empty)
			// This allows empty data/event fields but excludes ID-only events
			if hasDataOrEvent {
				s.event = event
				return true
			}
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Parse field
		if colonIndex := strings.Index(line, ":"); colonIndex != -1 {
			field := strings.TrimSpace(line[:colonIndex])
			value := strings.TrimSpace(line[colonIndex+1:])

			switch field {
			case "event":
				event.Event = value
				hasDataOrEvent = true
			case "data":
				if event.Data != "" {
					event.Data += "\n"
				}
				event.Data += value
				hasDataOrEvent = true
			case "id":
				event.ID = value
				// ID field doesn't make an event valid by itself
			}
		}
	}

	// Check for final event without trailing newline
	if hasDataOrEvent {
		s.event = event
		return true
	}

	s.err = s.scanner.Err()
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
