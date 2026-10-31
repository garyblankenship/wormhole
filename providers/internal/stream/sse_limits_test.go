package stream

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestSSEPhysicalLineLimit(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		lineLen int
		wantErr bool
	}{
		{name: "at limit", lineLen: maxSSEBufferBytes},
		{name: "one byte over", lineLen: maxSSEBufferBytes + 1, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := "data: " + strings.Repeat("x", tt.lineLen-len("data: ")) + "\n\n"

			scanner := NewSSEScanner(strings.NewReader(input))
			scanned := scanner.Scan()
			if tt.wantErr {
				assert.False(t, scanned)
				assert.ErrorIs(t, scanner.Err(), errSSEFrameTooLarge)
			} else {
				require.True(t, scanned)
				assert.NoError(t, scanner.Err())
			}

			parser := NewSSEParser(strings.NewReader(input))
			_, err := parser.Parse()
			if tt.wantErr {
				assert.ErrorIs(t, err, errSSEFrameTooLarge)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSSEAggregateMultilineDataLimit(t *testing.T) {
	t.Parallel()

	first := strings.Repeat("a", maxSSEBufferBytes/2)
	secondAtLimit := strings.Repeat("b", maxSSEBufferBytes-len(first)-1)

	for _, tt := range []struct {
		name    string
		second  string
		wantErr bool
	}{
		{name: "at limit", second: secondAtLimit},
		{name: "one byte over", second: secondAtLimit + "b", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := "data: " + first + "\ndata: " + tt.second + "\n\n"

			scanner := NewSSEScanner(strings.NewReader(input))
			scanned := scanner.Scan()
			if tt.wantErr {
				assert.False(t, scanned)
				assert.ErrorIs(t, scanner.Err(), errSSEFrameTooLarge)
			} else {
				require.True(t, scanned)
				assert.Len(t, scanner.Event().Data, maxSSEBufferBytes)
			}

			parser := NewSSEParser(strings.NewReader(input))
			event, err := parser.Parse()
			if tt.wantErr {
				assert.ErrorIs(t, err, errSSEFrameTooLarge)
			} else {
				require.NoError(t, err)
				assert.Len(t, event.Data, maxSSEBufferBytes)
			}
		})
	}
}

func TestSSEMultilineEmptyDataPreserved(t *testing.T) {
	t.Parallel()

	input := "data:\ndata: first\ndata:\ndata: last\ndata:\n\n"
	want := "first\n\nlast\n"
	parser := NewSSEParser(strings.NewReader(input))
	event, err := parser.Parse()
	require.NoError(t, err)
	assert.Equal(t, want, event.Data)

	scanner := NewSSEScanner(strings.NewReader(input))
	require.True(t, scanner.Scan())
	assert.Equal(t, want, scanner.Event().Data)
}

func TestSSEManyLineFrameLimit(t *testing.T) {
	t.Parallel()

	// Each line after the first adds 1024 bytes of data plus a separator.
	line := "data: " + strings.Repeat("x", 1023) + "\n"
	atLimit := strings.Repeat(line, 10240) + "\n"
	tooLarge := strings.Repeat(line, 10240) + "data: z\n\n"
	for _, tc := range []struct {
		name, input string
		wantErr     bool
	}{
		{name: "many lines at limit", input: atLimit},
		{name: "many lines over limit", input: tooLarge, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parser := NewSSEParser(strings.NewReader(tc.input))
			event, err := parser.Parse()
			scanner := NewSSEScanner(strings.NewReader(tc.input))
			if tc.wantErr {
				assert.ErrorIs(t, err, errSSEFrameTooLarge)
				assert.False(t, scanner.Scan())
				assert.ErrorIs(t, scanner.Err(), errSSEFrameTooLarge)
				return
			}
			require.NoError(t, err)
			assert.Len(t, event.Data, maxSSEBufferBytes-1)
			require.True(t, scanner.Scan())
			assert.Equal(t, event.Data, scanner.Event().Data)
		})
	}
}

func TestProcessSSEOversizedFrameReportsOnceAndClosesBody(t *testing.T) {
	t.Parallel()

	input := "data: " + strings.Repeat("x", maxSSEBufferBytes) + "\n\n"
	closed := make(chan struct{})
	body := &trackingCloser{Reader: strings.NewReader(input), closed: closed}
	transformer := func(data []byte) (*types.TextChunk, error) {
		return &types.TextChunk{Text: string(data)}, nil
	}

	var chunks []types.TextChunk
	for chunk := range ProcessSSE(context.Background(), body, transformer, 1) {
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 1)
	assert.True(t, errors.Is(chunks[0].Error, errSSEFrameTooLarge))
	select {
	case <-closed:
	default:
		t.Fatal("response body was not closed")
	}
}
