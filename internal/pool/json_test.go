package pool

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalAndReturn(t *testing.T) {
	buf, err := Marshal(map[string]any{"name": "wormhole", "count": 2})
	require.NoError(t, err)
	require.JSONEq(t, `{"name":"wormhole","count":2}`, string(buf))
	Return(buf)
	Return(nil)
}

func TestMarshalAllocatesWhenPooledSliceIsTooSmall(t *testing.T) {
	Return(make([]byte, 0, 1))

	buf, err := Marshal(map[string]string{"long": "this is longer than one byte"})
	require.NoError(t, err)
	assert.Greater(t, cap(buf), 1)
	Return(buf)
}

func TestMarshalReturnsEncodeError(t *testing.T) {
	_, err := Marshal(map[string]any{"bad": func() {}})
	require.Error(t, err)
}

func TestMarshalToString(t *testing.T) {
	got, err := MarshalToString([]string{"a", "b"})
	require.NoError(t, err)

	var decoded []string
	require.NoError(t, json.Unmarshal([]byte(got), &decoded))
	assert.Equal(t, []string{"a", "b"}, decoded)
}
