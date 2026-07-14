package stream

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSEScannerHandlesLargeLine(t *testing.T) {
	t.Parallel()

	// Payload well beyond bufio's default 64 KB token cap.
	largePayload := strings.Repeat("x", 200*1024)
	input := "data: " + largePayload + "\n\n"

	scanner := NewSSEScanner(strings.NewReader(input))

	require.True(t, scanner.Scan(), "scan of a >64KB SSE line must succeed")
	require.NoError(t, scanner.Err(), "no ErrTooLong on large line")

	ev := scanner.Event()
	require.NotNil(t, ev)
	assert.Equal(t, largePayload, ev.Data, "full large payload must be delivered intact")
}
