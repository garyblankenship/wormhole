package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolResultMessage_WithError(t *testing.T) {
	t.Parallel()
	m := NewToolResultMessage("call_1", "ok").WithError("boom")
	assert.Equal(t, "boom", m.Error)
	// Error field must NOT appear on the OpenAI-format wire.
	out, err := json.Marshal(m)
	require.NoError(t, err)
	assert.NotContains(t, string(out), `"error"`)
}
