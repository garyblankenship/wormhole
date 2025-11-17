package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// SSEParser parses Server-Sent Events streams
type SSEParser struct {
	reader *bufio.Reader
}

// NewSSEParser creates a new SSE parser
func NewSSEParser(r io.Reader) *SSEParser {
	return &SSEParser{
		reader: bufio.NewReader(r),
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
			return nil, returnErr
		}
		if shouldReturn {
			return event, nil
		}

		// Skip empty lines and comments, but check for EOF first
		if p.shouldSkip(line) {
			// If we hit EOF while skipping empty lines, return EOF if no event data
			if eof && !p.hasEventData(event) {
				return nil, io.EOF
			}
			continue
		}

		// Parse and apply field to event
		if err := p.parseField(line, event); err != nil {
			continue // Invalid field format, skip
		}

		// Return event if we reached EOF after processing
		if eof {
			return event, nil
		}
	}
}

// readLine reads next line and handles EOF
func (p *SSEParser) readLine() (string, bool, error) {
	line, err := p.reader.ReadString('\n')

	// Handle EOF with remaining content
	if err == io.EOF && line != "" {
		return strings.TrimSpace(line), true, nil
	}

	// Handle other errors
	if err != nil && err != io.EOF {
		return "", false, err
	}

	return strings.TrimSpace(line), err == io.EOF, nil
}

// shouldReturn checks if event is complete and should be returned
// Returns (shouldReturn bool, returnError error)
func (p *SSEParser) shouldReturnWithError(line string, event *SSEEvent, isEOF bool) (bool, error) {
	// Empty line signals end of event
	if line == "" {
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
func (p *SSEParser) shouldSkip(line string) bool {
	return line == "" || strings.HasPrefix(line, ":")
}

// hasEventData checks if event has meaningful data
func (p *SSEParser) hasEventData(event *SSEEvent) bool {
	return event.Data != "" || event.Event != ""
}

// parseField parses a field line and updates the event
func (p *SSEParser) parseField(line string, event *SSEEvent) error {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return fmt.Errorf("invalid field format")
	}

	field := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	p.applyField(field, value, event)
	return nil
}

// applyField applies a parsed field to the event
func (p *SSEParser) applyField(field, value string, event *SSEEvent) {
	switch field {
	case "event":
		event.Event = value
	case "data":
		if event.Data != "" {
			event.Data += "\n"
		}
		event.Data += value
	case "id":
		event.ID = value
	}
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

// Process processes the stream and sends chunks to the channel
func (p *StreamProcessor) Process(chunks chan<- types.TextChunk) {
	defer close(chunks)

	for {
		event, err := p.parser.Parse()
		if err != nil {
			if err != io.EOF {
				chunks <- types.TextChunk{Error: err}
			}
			return
		}

		// Skip non-data events
		if event.Data == "" {
			continue
		}

		// Handle [DONE] marker
		if event.Data == "[DONE]" {
			return
		}

		// Transform the data
		chunk, err := p.transformer([]byte(event.Data))
		if err != nil {
			chunks <- types.TextChunk{Error: fmt.Errorf("failed to parse chunk: %w", err)}
			return
		}

		if chunk != nil {
			chunks <- *chunk
		}
	}
}

// MergeTextChunks merges text chunks into a complete response
func MergeTextChunks(chunks []types.TextChunk) *types.TextResponse {
	var text strings.Builder
	var toolCalls []types.ToolCall
	var usage *types.Usage
	var finishReason types.FinishReason
	var id, model string

	for _, chunk := range chunks {
		if chunk.ID != "" {
			id = chunk.ID
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.Text != "" {
			text.WriteString(chunk.Text)
		}
		if chunk.FinishReason != nil {
			finishReason = *chunk.FinishReason
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.ToolCall != nil {
			toolCalls = append(toolCalls, *chunk.ToolCall)
		}
	}

	return &types.TextResponse{
		ID:           id,
		Model:        model,
		Text:         text.String(),
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
		Usage:        usage,
	}
}

// JSONStreamParser parses JSON responses from a stream
type JSONStreamParser struct {
	decoder *json.Decoder
}

// NewJSONStreamParser creates a new JSON stream parser
func NewJSONStreamParser(r io.Reader) *JSONStreamParser {
	return &JSONStreamParser{
		decoder: json.NewDecoder(r),
	}
}

// Parse reads and parses the next JSON object
func (p *JSONStreamParser) Parse(v any) error {
	return p.decoder.Decode(v)
}
