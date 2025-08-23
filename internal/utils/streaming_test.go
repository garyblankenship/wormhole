package utils

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSEParser_Creation(t *testing.T) {
	reader := strings.NewReader("test")
	parser := NewSSEParser(reader)

	assert.NotNil(t, parser)
	assert.NotNil(t, parser.reader)
}

func TestSSEParser_BasicParsing(t *testing.T) {
	t.Run("simple event", func(t *testing.T) {
		input := `event: message
data: Hello World
id: 123

`
		parser := NewSSEParser(strings.NewReader(input))

		event, err := parser.Parse()
		require.NoError(t, err)
		assert.Equal(t, "message", event.Event)
		assert.Equal(t, "Hello World", event.Data)
		assert.Equal(t, "123", event.ID)
	})

	t.Run("data only", func(t *testing.T) {
		input := `data: Just data

`
		parser := NewSSEParser(strings.NewReader(input))

		event, err := parser.Parse()
		require.NoError(t, err)
		assert.Equal(t, "", event.Event)
		assert.Equal(t, "Just data", event.Data)
		assert.Equal(t, "", event.ID)
	})

	t.Run("empty event", func(t *testing.T) {
		input := `

`
		parser := NewSSEParser(strings.NewReader(input))

		_, err := parser.Parse()
		assert.Equal(t, io.EOF, err)
	})
}

func TestSSEParser_MultilineData(t *testing.T) {
	input := `data: Line 1
data: Line 2
data: Line 3

`
	parser := NewSSEParser(strings.NewReader(input))

	event, err := parser.Parse()
	require.NoError(t, err)
	expected := "Line 1\nLine 2\nLine 3"
	assert.Equal(t, expected, event.Data)
}

func TestSSEParser_Comments(t *testing.T) {
	input := `: This is a comment
data: Real data
: Another comment

`
	parser := NewSSEParser(strings.NewReader(input))

	event, err := parser.Parse()
	require.NoError(t, err)
	assert.Equal(t, "Real data", event.Data)
}

func TestSSEParser_ErrorHandling(t *testing.T) {
	t.Run("EOF", func(t *testing.T) {
		parser := NewSSEParser(strings.NewReader(""))

		_, err := parser.Parse()
		assert.Equal(t, io.EOF, err)
	})

	t.Run("final line without newline", func(t *testing.T) {
		input := `data: Final line`
		parser := NewSSEParser(strings.NewReader(input))

		event, err := parser.Parse()
		require.NoError(t, err)
		assert.Equal(t, "Final line", event.Data)
	})
}

func TestStreamProcessor_Creation(t *testing.T) {
	reader := strings.NewReader("test")
	transformer := func(data []byte) (*types.TextChunk, error) {
		return &types.TextChunk{Text: string(data)}, nil
	}

	processor := NewStreamProcessor(reader, transformer)

	assert.NotNil(t, processor)
	assert.NotNil(t, processor.parser)
	assert.NotNil(t, processor.transformer)
}

func TestStreamProcessor_Processing(t *testing.T) {
	t.Run("successful processing", func(t *testing.T) {
		input := `data: {"text": "Hello"}

data: {"text": " World"}

data: [DONE]

`

		transformer := func(data []byte) (*types.TextChunk, error) {
			if string(data) == "[DONE]" {
				return nil, nil
			}

			var parsed struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil {
				return nil, err
			}

			return &types.TextChunk{
				Text: parsed.Text,
				ID:   "test-id",
			}, nil
		}

		processor := NewStreamProcessor(strings.NewReader(input), transformer)
		chunks := make(chan types.TextChunk, 10)

		go processor.Process(chunks)

		// Collect chunks
		var received []types.TextChunk
		for chunk := range chunks {
			received = append(received, chunk)
		}

		assert.Len(t, received, 2)
		assert.Equal(t, "Hello", received[0].Text)
		assert.Equal(t, " World", received[1].Text)
		assert.Equal(t, "test-id", received[0].ID)
		assert.Nil(t, received[0].Error)
	})

	t.Run("transformer error", func(t *testing.T) {
		input := `data: invalid json

`

		transformer := func(data []byte) (*types.TextChunk, error) {
			return nil, errors.New("transformer error")
		}

		processor := NewStreamProcessor(strings.NewReader(input), transformer)
		chunks := make(chan types.TextChunk, 10)

		go processor.Process(chunks)

		// Should receive error chunk
		chunk := <-chunks
		assert.Error(t, chunk.Error)
		assert.Contains(t, chunk.Error.Error(), "failed to parse chunk")

		// Channel should be closed
		_, ok := <-chunks
		assert.False(t, ok)
	})

	t.Run("parser error", func(t *testing.T) {
		errorReader := &errorReader{}
		transformer := func(data []byte) (*types.TextChunk, error) {
			return &types.TextChunk{Text: string(data)}, nil
		}

		processor := NewStreamProcessor(errorReader, transformer)
		chunks := make(chan types.TextChunk, 10)

		go processor.Process(chunks)

		// Should receive error chunk
		chunk := <-chunks
		assert.Error(t, chunk.Error)

		// Channel should be closed
		_, ok := <-chunks
		assert.False(t, ok)
	})

	t.Run("DONE marker handling", func(t *testing.T) {
		input := `data: {"text": "Hello"}

data: [DONE]

`

		transformer := func(data []byte) (*types.TextChunk, error) {
			if string(data) == "[DONE]" {
				return nil, nil
			}
			return &types.TextChunk{Text: "processed"}, nil
		}

		processor := NewStreamProcessor(strings.NewReader(input), transformer)
		chunks := make(chan types.TextChunk, 10)

		go processor.Process(chunks)

		// Collect chunks
		var received []types.TextChunk
		for chunk := range chunks {
			received = append(received, chunk)
		}

		// Should only receive the first chunk, DONE should stop processing
		assert.Len(t, received, 1)
		assert.Equal(t, "processed", received[0].Text)
	})

	t.Run("empty data events skipped", func(t *testing.T) {
		input := `event: ping

data: {"text": "Hello"}

event: heartbeat

data: {"text": "World"}

`

		transformer := func(data []byte) (*types.TextChunk, error) {
			var parsed struct {
				Text string `json:"text"`
			}
			json.Unmarshal(data, &parsed)
			return &types.TextChunk{Text: parsed.Text}, nil
		}

		processor := NewStreamProcessor(strings.NewReader(input), transformer)
		chunks := make(chan types.TextChunk, 10)

		go processor.Process(chunks)

		// Collect chunks
		var received []types.TextChunk
		timeout := time.After(100 * time.Millisecond)

	loop:
		for {
			select {
			case chunk, ok := <-chunks:
				if !ok {
					break loop
				}
				received = append(received, chunk)
			case <-timeout:
				break loop
			}
		}

		// Should receive 2 chunks, empty events should be skipped
		assert.Len(t, received, 2)
		assert.Equal(t, "Hello", received[0].Text)
		assert.Equal(t, "World", received[1].Text)
	})
}

func TestMergeTextChunks(t *testing.T) {
	t.Run("merge simple chunks", func(t *testing.T) {
		chunks := []types.TextChunk{
			{ID: "test-1", Model: "gpt-5", Text: "Hello"},
			{Text: " "},
			{Text: "World"},
		}

		response := MergeTextChunks(chunks)

		assert.Equal(t, "test-1", response.ID)
		assert.Equal(t, "gpt-5", response.Model)
		assert.Equal(t, "Hello World", response.Text)
		assert.Empty(t, response.ToolCalls)
		assert.Equal(t, types.FinishReason(""), response.FinishReason)
		assert.Nil(t, response.Usage)
	})

	t.Run("merge with finish reason and usage", func(t *testing.T) {
		finishReason := types.FinishReasonStop
		chunks := []types.TextChunk{
			{Text: "Hello"},
			{Text: " World"},
			{
				FinishReason: &finishReason,
				Usage: &types.Usage{
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				},
			},
		}

		response := MergeTextChunks(chunks)

		assert.Equal(t, "Hello World", response.Text)
		assert.Equal(t, types.FinishReasonStop, response.FinishReason)
		assert.NotNil(t, response.Usage)
		assert.Equal(t, 10, response.Usage.PromptTokens)
		assert.Equal(t, 5, response.Usage.CompletionTokens)
		assert.Equal(t, 15, response.Usage.TotalTokens)
	})

	t.Run("merge with tool calls", func(t *testing.T) {
		toolCall1 := types.ToolCall{
			ID:   "call-1",
			Type: "function",
			Function: &types.ToolCallFunction{
				Name:      "get_weather",
				Arguments: `{"location": "NYC"}`,
			},
		}

		toolCall2 := types.ToolCall{
			ID:   "call-2",
			Type: "function",
			Function: &types.ToolCallFunction{
				Name:      "get_time",
				Arguments: `{}`,
			},
		}

		chunks := []types.TextChunk{
			{Text: "I'll help you with that."},
			{ToolCall: &toolCall1},
			{ToolCall: &toolCall2},
		}

		response := MergeTextChunks(chunks)

		assert.Equal(t, "I'll help you with that.", response.Text)
		assert.Len(t, response.ToolCalls, 2)
		assert.Equal(t, "call-1", response.ToolCalls[0].ID)
		assert.Equal(t, "call-2", response.ToolCalls[1].ID)
	})

	t.Run("empty chunks", func(t *testing.T) {
		chunks := []types.TextChunk{}

		response := MergeTextChunks(chunks)

		assert.Equal(t, "", response.ID)
		assert.Equal(t, "", response.Model)
		assert.Equal(t, "", response.Text)
		assert.Empty(t, response.ToolCalls)
		assert.Equal(t, types.FinishReason(""), response.FinishReason)
		assert.Nil(t, response.Usage)
	})

	t.Run("chunks with errors ignored", func(t *testing.T) {
		chunks := []types.TextChunk{
			{Text: "Hello"},
			{Error: errors.New("some error")},
			{Text: " World"},
		}

		response := MergeTextChunks(chunks)

		// Error chunks should not affect the merged response
		assert.Equal(t, "Hello World", response.Text)
	})

	t.Run("overwrite metadata with latest", func(t *testing.T) {
		chunks := []types.TextChunk{
			{ID: "old-id", Model: "old-model", Text: "Hello"},
			{ID: "new-id", Model: "new-model", Text: " World"},
		}

		response := MergeTextChunks(chunks)

		// Should use the latest non-empty values
		assert.Equal(t, "new-id", response.ID)
		assert.Equal(t, "new-model", response.Model)
		assert.Equal(t, "Hello World", response.Text)
	})
}

func TestJSONStreamParser_Creation(t *testing.T) {
	reader := strings.NewReader(`{"test": "data"}`)
	parser := NewJSONStreamParser(reader)

	assert.NotNil(t, parser)
	assert.NotNil(t, parser.decoder)
}

func TestJSONStreamParser_Parsing(t *testing.T) {
	t.Run("parse single object", func(t *testing.T) {
		input := `{"name": "test", "value": 123}`
		parser := NewJSONStreamParser(strings.NewReader(input))

		var result map[string]any
		err := parser.Parse(&result)

		require.NoError(t, err)
		assert.Equal(t, "test", result["name"])
		assert.Equal(t, float64(123), result["value"]) // JSON numbers are float64
	})

	t.Run("parse multiple objects", func(t *testing.T) {
		input := `{"id": 1}
{"id": 2}
{"id": 3}`
		parser := NewJSONStreamParser(strings.NewReader(input))

		var results []map[string]any

		for {
			var obj map[string]any
			err := parser.Parse(&obj)
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			results = append(results, obj)
		}

		assert.Len(t, results, 3)
		assert.Equal(t, float64(1), results[0]["id"])
		assert.Equal(t, float64(2), results[1]["id"])
		assert.Equal(t, float64(3), results[2]["id"])
	})

	t.Run("parse into struct", func(t *testing.T) {
		type TestStruct struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}

		input := `{"name": "test", "value": 456}`
		parser := NewJSONStreamParser(strings.NewReader(input))

		var result TestStruct
		err := parser.Parse(&result)

		require.NoError(t, err)
		assert.Equal(t, "test", result.Name)
		assert.Equal(t, 456, result.Value)
	})

	t.Run("parse error with invalid JSON", func(t *testing.T) {
		input := `{"name": "test", "value": 123,}` // Invalid trailing comma
		parser := NewJSONStreamParser(strings.NewReader(input))

		var result map[string]any
		err := parser.Parse(&result)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid character")
	})

	t.Run("EOF on empty input", func(t *testing.T) {
		parser := NewJSONStreamParser(strings.NewReader(""))

		var result map[string]any
		err := parser.Parse(&result)

		assert.Equal(t, io.EOF, err)
	})
}

func TestStreamProcessor_RealWorldScenarios(t *testing.T) {
	t.Run("OpenAI-style streaming", func(t *testing.T) {
		input := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":" "}}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"World"}}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"finish_reason":"stop"}]}

data: [DONE]

`

		transformer := func(data []byte) (*types.TextChunk, error) {
			if string(data) == "[DONE]" {
				return nil, nil
			}

			var response struct {
				ID      string `json:"id"`
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}

			if err := json.Unmarshal(data, &response); err != nil {
				return nil, err
			}

			chunk := &types.TextChunk{
				ID: response.ID,
			}

			if len(response.Choices) > 0 {
				choice := response.Choices[0]
				chunk.Text = choice.Delta.Content

				if choice.FinishReason != nil {
					reason := types.FinishReason(*choice.FinishReason)
					chunk.FinishReason = &reason
				}
			}

			return chunk, nil
		}

		processor := NewStreamProcessor(strings.NewReader(input), transformer)
		chunks := make(chan types.TextChunk, 10)

		go processor.Process(chunks)

		// Collect chunks
		var received []types.TextChunk
		for chunk := range chunks {
			received = append(received, chunk)
		}

		assert.Len(t, received, 4)
		assert.Equal(t, "Hello", received[0].Text)
		assert.Equal(t, " ", received[1].Text)
		assert.Equal(t, "World", received[2].Text)
		assert.Equal(t, "", received[3].Text)
		assert.NotNil(t, received[3].FinishReason)
		assert.Equal(t, types.FinishReason("stop"), *received[3].FinishReason)

		// Merge chunks to verify complete response
		response := MergeTextChunks(received)
		assert.Equal(t, "Hello World", response.Text)
		assert.Equal(t, "chatcmpl-123", response.ID)
		assert.Equal(t, types.FinishReason("stop"), response.FinishReason)
	})

	t.Run("Anthropic-style streaming", func(t *testing.T) {
		input := `event: message_start
data: {"type":"message_start","message":{"id":"msg_123"}}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":" World"}}

event: message_stop
data: {"type":"message_stop"}

`

		transformer := func(data []byte) (*types.TextChunk, error) {
			var event struct {
				Type    string `json:"type"`
				Message *struct {
					ID string `json:"id"`
				} `json:"message,omitempty"`
				Delta *struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta,omitempty"`
			}

			if err := json.Unmarshal(data, &event); err != nil {
				return nil, err
			}

			chunk := &types.TextChunk{}

			switch event.Type {
			case "message_start":
				if event.Message != nil {
					chunk.ID = event.Message.ID
				}
			case "content_block_delta":
				if event.Delta != nil && event.Delta.Type == "text_delta" {
					chunk.Text = event.Delta.Text
				}
			case "message_stop":
				reason := types.FinishReasonStop
				chunk.FinishReason = &reason
			}

			return chunk, nil
		}

		processor := NewStreamProcessor(strings.NewReader(input), transformer)
		chunks := make(chan types.TextChunk, 10)

		go processor.Process(chunks)

		// Collect chunks
		var received []types.TextChunk
		timeout := time.After(100 * time.Millisecond)

	loop:
		for {
			select {
			case chunk, ok := <-chunks:
				if !ok {
					break loop
				}
				received = append(received, chunk)
			case <-timeout:
				break loop
			}
		}

		assert.Len(t, received, 4) // message_start, 2 deltas, message_stop

		// Merge and verify
		response := MergeTextChunks(received)
		assert.Equal(t, "Hello World", response.Text)
		assert.Equal(t, "msg_123", response.ID)
		assert.Equal(t, types.FinishReasonStop, response.FinishReason)
	})
}

func TestStreamProcessor_EdgeCases(t *testing.T) {
	t.Run("transformer returns nil chunk", func(t *testing.T) {
		input := `data: skip-this

data: {"text": "process-this"}

`

		transformer := func(data []byte) (*types.TextChunk, error) {
			if string(data) == "skip-this" {
				return nil, nil // Skip this chunk
			}
			return &types.TextChunk{Text: "processed"}, nil
		}

		processor := NewStreamProcessor(strings.NewReader(input), transformer)
		chunks := make(chan types.TextChunk, 10)

		go processor.Process(chunks)

		// Should only receive one chunk
		chunk := <-chunks
		assert.Equal(t, "processed", chunk.Text)

		// Channel should close after processing
		_, ok := <-chunks
		assert.False(t, ok)
	})

	t.Run("concurrent chunk processing", func(t *testing.T) {
		input := `data: chunk1

data: chunk2

data: chunk3

data: [DONE]

`

		transformer := func(data []byte) (*types.TextChunk, error) {
			return &types.TextChunk{Text: string(data)}, nil
		}

		processor := NewStreamProcessor(strings.NewReader(input), transformer)
		chunks := make(chan types.TextChunk, 10)

		go processor.Process(chunks)

		// Read chunks concurrently
		received := make(map[string]bool)
		for chunk := range chunks {
			received[chunk.Text] = true
		}

		assert.True(t, received["chunk1"])
		assert.True(t, received["chunk2"])
		assert.True(t, received["chunk3"])
		assert.Len(t, received, 3) // Should not include [DONE]
	})
}
