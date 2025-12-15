package utils

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test constants for SSE event inputs
const (
	testSSECompleteEvent = `event: message
data: Hello World
id: 123

`
	testSSEDataOnlyEvent = `data: Just data

`
)

func TestSSEEvent_Structure(t *testing.T) {
	event := &SSEEvent{
		Event: "message",
		Data:  "test data",
		ID:    "123",
	}

	assert.Equal(t, "message", event.Event)
	assert.Equal(t, "test data", event.Data)
	assert.Equal(t, "123", event.ID)
}

func TestSSEScanner_Creation(t *testing.T) {
	reader := strings.NewReader("test")
	scanner := NewSSEScanner(reader)

	assert.NotNil(t, scanner)
	assert.NotNil(t, scanner.scanner)
	assert.Nil(t, scanner.event)
	assert.Nil(t, scanner.err)
}

func TestSSEScanner_BasicEventParsing(t *testing.T) {
	t.Run("complete event", func(t *testing.T) {
		scanner := NewSSEScanner(strings.NewReader(testSSECompleteEvent))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "message", event.Event)
		assert.Equal(t, "Hello World", event.Data)
		assert.Equal(t, "123", event.ID)

		assert.False(t, scanner.Scan())
		assert.NoError(t, scanner.Err())
	})

	t.Run("data only event", func(t *testing.T) {
		scanner := NewSSEScanner(strings.NewReader(testSSEDataOnlyEvent))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "", event.Event)
		assert.Equal(t, "Just data", event.Data)
		assert.Equal(t, "", event.ID)
	})

	t.Run("event only", func(t *testing.T) {
		input := `event: ping

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "ping", event.Event)
		assert.Equal(t, "", event.Data)
		assert.Equal(t, "", event.ID)
	})

	t.Run("id only", func(t *testing.T) {
		input := `id: 456

`
		scanner := NewSSEScanner(strings.NewReader(input))

		// An event with only ID is not considered valid by this implementation
		// (requires Data or Event to be non-empty)
		assert.False(t, scanner.Scan())
		assert.NoError(t, scanner.Err())
	})
}

func TestSSEScanner_MultilineData(t *testing.T) {
	t.Run("multiple data lines", func(t *testing.T) {
		input := `data: First line
data: Second line
data: Third line

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		expected := "First line\nSecond line\nThird line"
		assert.Equal(t, expected, event.Data)
	})

	t.Run("empty data lines", func(t *testing.T) {
		input := `data: First line
data: 
data: Third line

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		expected := "First line\n\nThird line"
		assert.Equal(t, expected, event.Data)
	})
}

func TestSSEScanner_Comments(t *testing.T) {
	t.Run("comment lines ignored", func(t *testing.T) {
		input := `: This is a comment
data: Real data
: Another comment
event: test

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "test", event.Event)
		assert.Equal(t, "Real data", event.Data)
	})

	t.Run("comment at start of line", func(t *testing.T) {
		input := `:comment
data: test

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "test", event.Data)
	})
}

func TestSSEScanner_FieldParsing(t *testing.T) {
	t.Run("fields with spaces around colon", func(t *testing.T) {
		input := `event : message
data : Hello World
id : 123

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "message", event.Event)
		assert.Equal(t, "Hello World", event.Data)
		assert.Equal(t, "123", event.ID)
	})

	t.Run("fields with extra spaces in values", func(t *testing.T) {
		input := `event:   message   
data:   Hello World   
id:   123   

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "message", event.Event)
		assert.Equal(t, "Hello World", event.Data)
		assert.Equal(t, "123", event.ID)
	})

	t.Run("fields with multiple colons in value", func(t *testing.T) {
		input := `data: https://example.com:8080/path
event: url:test

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "url:test", event.Event)
		assert.Equal(t, "https://example.com:8080/path", event.Data)
	})

	t.Run("unknown fields ignored", func(t *testing.T) {
		input := `unknown: value
data: test data
retry: 5000

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "", event.Event)
		assert.Equal(t, "test data", event.Data)
		assert.Equal(t, "", event.ID)
	})
}

func TestSSEScanner_MultipleEvents(t *testing.T) {
	input := `event: first
data: First event data

event: second
data: Second event data
id: 2

data: Third event (no event type)

`
	scanner := NewSSEScanner(strings.NewReader(input))

	// First event
	assert.True(t, scanner.Scan())
	event1 := scanner.Event()
	assert.Equal(t, "first", event1.Event)
	assert.Equal(t, "First event data", event1.Data)
	assert.Equal(t, "", event1.ID)

	// Second event
	assert.True(t, scanner.Scan())
	event2 := scanner.Event()
	assert.Equal(t, "second", event2.Event)
	assert.Equal(t, "Second event data", event2.Data)
	assert.Equal(t, "2", event2.ID)

	// Third event
	assert.True(t, scanner.Scan())
	event3 := scanner.Event()
	assert.Equal(t, "", event3.Event)
	assert.Equal(t, "Third event (no event type)", event3.Data)
	assert.Equal(t, "", event3.ID)

	// No more events
	assert.False(t, scanner.Scan())
	assert.NoError(t, scanner.Err())
}

func TestSSEScanner_EdgeCases(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		scanner := NewSSEScanner(strings.NewReader(""))

		assert.False(t, scanner.Scan())
		assert.NoError(t, scanner.Err())
		assert.Nil(t, scanner.Event())
	})

	t.Run("only empty lines", func(t *testing.T) {
		input := `


`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.False(t, scanner.Scan())
		assert.NoError(t, scanner.Err())
	})

	t.Run("only comments", func(t *testing.T) {
		input := `: comment 1
: comment 2

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.False(t, scanner.Scan())
		assert.NoError(t, scanner.Err())
	})

	t.Run("event without trailing newline", func(t *testing.T) {
		input := `data: Final event without newline`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "Final event without newline", event.Data)

		assert.False(t, scanner.Scan())
		assert.NoError(t, scanner.Err())
	})

	t.Run("malformed lines without colon", func(t *testing.T) {
		input := `data: good data
malformed line
event: test

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "test", event.Event)
		assert.Equal(t, "good data", event.Data)
	})

	t.Run("field with empty value", func(t *testing.T) {
		input := `data:
event:
id:

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "", event.Event)
		assert.Equal(t, "", event.Data)
		assert.Equal(t, "", event.ID)
	})
}

func TestSSEScanner_RealWorldExamples(t *testing.T) {
	t.Run("OpenAI streaming format", func(t *testing.T) {
		input := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"},"index":0}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":" World"},"index":0}]}

data: [DONE]

`
		scanner := NewSSEScanner(strings.NewReader(input))

		// First chunk
		assert.True(t, scanner.Scan())
		event1 := scanner.Event()
		assert.Contains(t, event1.Data, "Hello")
		assert.Contains(t, event1.Data, "chatcmpl-123")

		// Second chunk
		assert.True(t, scanner.Scan())
		event2 := scanner.Event()
		assert.Contains(t, event2.Data, " World")

		// Done marker
		assert.True(t, scanner.Scan())
		event3 := scanner.Event()
		assert.Equal(t, "[DONE]", event3.Data)

		assert.False(t, scanner.Scan())
	})

	t.Run("Anthropic streaming format", func(t *testing.T) {
		input := `event: message_start
data: {"type":"message_start","message":{"id":"msg_123","type":"message"}}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}

event: message_stop
data: {"type":"message_stop"}

`
		scanner := NewSSEScanner(strings.NewReader(input))

		// Message start
		assert.True(t, scanner.Scan())
		event1 := scanner.Event()
		assert.Equal(t, "message_start", event1.Event)
		assert.Contains(t, event1.Data, "message_start")

		// Content delta
		assert.True(t, scanner.Scan())
		event2 := scanner.Event()
		assert.Equal(t, "content_block_delta", event2.Event)
		assert.Contains(t, event2.Data, "Hello")

		// Message stop
		assert.True(t, scanner.Scan())
		event3 := scanner.Event()
		assert.Equal(t, "message_stop", event3.Event)

		assert.False(t, scanner.Scan())
	})

	t.Run("event with retry field", func(t *testing.T) {
		input := `retry: 3000
event: message
data: Connection retry test
id: retry-1

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "message", event.Event)
		assert.Equal(t, "Connection retry test", event.Data)
		assert.Equal(t, "retry-1", event.ID)
	})
}

func TestSSEScanner_Whitespace(t *testing.T) {
	t.Run("whitespace trimming", func(t *testing.T) {
		input := `  event:  message  
  data:  Hello World  
  id:  123  

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "message", event.Event)
		assert.Equal(t, "Hello World", event.Data)
		assert.Equal(t, "123", event.ID)
	})

	t.Run("tabs and mixed whitespace", func(t *testing.T) {
		input := "\tevent:\tmessage\t\n\tdata:\tHello\tWorld\t\n\n"
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "message", event.Event)
		assert.Equal(t, "Hello\tWorld", event.Data) // Internal tabs preserved
	})
}

func TestSSEScanner_JSON(t *testing.T) {
	t.Run("JSON data parsing", func(t *testing.T) {
		jsonData := `{"type":"completion","text":"Hello","id":123}`
		input := `event: completion
data: ` + jsonData + `
id: json-test

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "completion", event.Event)
		assert.Equal(t, jsonData, event.Data)
		assert.Equal(t, "json-test", event.ID)
	})

	t.Run("multiline JSON data", func(t *testing.T) {
		input := `data: {
data:   "text": "Hello",
data:   "id": 123
data: }

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		// After trimming spaces, this is the correct result
		expected := "{\n\"text\": \"Hello\",\n\"id\": 123\n}"
		assert.Equal(t, expected, event.Data)
	})
}

// errorReader simulates a reader that returns an error
type errorReader struct{}

func (e errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func TestSSEScanner_ErrorHandling(t *testing.T) {
	t.Run("reader error", func(t *testing.T) {
		scanner := NewSSEScanner(&errorReader{})

		assert.False(t, scanner.Scan())
		assert.Error(t, scanner.Err())
		assert.Contains(t, scanner.Err().Error(), "read error")
	})

	t.Run("partial read before error", func(t *testing.T) {
		// Create a reader that provides some data then errors
		data := "data: partial data\n"
		reader := io.MultiReader(
			strings.NewReader(data),
			&errorReader{},
		)

		scanner := NewSSEScanner(reader)

		// Should be able to read the partial data
		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, "partial data", event.Data)

		// Next scan should return false with error
		assert.False(t, scanner.Scan())
		assert.Error(t, scanner.Err())
	})
}

func TestSSEScanner_LargeData(t *testing.T) {
	t.Run("large data field", func(t *testing.T) {
		// Create a large data field
		largeData := strings.Repeat("x", 10000)
		input := `data: ` + largeData + `

`
		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, largeData, event.Data)
		assert.Len(t, event.Data, 10000)
	})

	t.Run("many data lines", func(t *testing.T) {
		input := ""
		expected := ""

		// Add 100 data lines
		for i := 0; i < 100; i++ {
			line := "Line " + string(rune('A'+i%26))
			input += "data: " + line + "\n"
			if i > 0 {
				expected += "\n"
			}
			expected += line
		}
		input += "\n"

		scanner := NewSSEScanner(strings.NewReader(input))

		assert.True(t, scanner.Scan())
		event := scanner.Event()
		assert.Equal(t, expected, event.Data)
	})
}
