package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/prism-php/prism-go/pkg/types"
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
		line, err := p.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && line != "" {
				// Process the last line if it doesn't end with newline
			} else {
				return nil, err
			}
		}

		line = strings.TrimSpace(line)

		// Empty line signals end of event
		if line == "" {
			if event.Data != "" || event.Event != "" {
				return event, nil
			}
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Parse field
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}

		field := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

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
			// Retry field is not used in SSEEvent
		}
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
func (p *JSONStreamParser) Parse(v interface{}) error {
	return p.decoder.Decode(v)
}
