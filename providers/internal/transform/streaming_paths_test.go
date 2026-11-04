package transform

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustDecodeMap(t *testing.T, data string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(data), &m))
	return m
}

func TestCompilePathSegments(t *testing.T) {
	t.Parallel()

	segments := compilePath("choices.0.delta.content")
	require.Len(t, segments, 4)
	require.Equal(t, pathSegment{key: "choices"}, segments[0])
	require.Equal(t, pathSegment{index: 0, isIndex: true}, segments[1])
	require.Equal(t, pathSegment{key: "delta"}, segments[2])
	require.Equal(t, pathSegment{key: "content"}, segments[3])

	require.Nil(t, compilePath(""))
	require.Len(t, compilePath("usage"), 1)
	require.Equal(t, 10, compilePath("items.10")[1].index)
}

func TestGetFieldSemantics(t *testing.T) {
	t.Parallel()

	data := mustDecodeMap(t, `{"choices":[{"delta":{"content":"hi"}}],"usage":{"cached":2}}`)

	require.Equal(t, "hi", getField(data, compilePath("choices.0.delta.content")))
	usage, ok := getField(data, compilePath("usage")).(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(2), usage["cached"])
	require.Equal(t, float64(2), getField(data, compilePath("usage.cached")))

	// Misses: out-of-range index, wrong types, absent keys.
	require.Nil(t, getField(data, compilePath("choices.1.delta.content")))
	require.Nil(t, getField(data, compilePath("choices.delta")))
	require.Nil(t, getField(data, compilePath("usage.missing")))
	require.Nil(t, getField(data, compilePath("missing")))
	require.Nil(t, getField(nil, compilePath("usage")))
}

func TestParseChunkWithMapReturnsRoot(t *testing.T) {
	t.Parallel()

	transformer := NewOpenAIStreamingTransformer()

	chunk, root, err := transformer.ParseChunkWithMap([]byte(`{"id":"chatcmpl-1","model":"gpt-test","choices":[{"delta":{"content":"hi"}}],"usage":{"future_detail":true}}`))
	require.NoError(t, err)
	require.NotNil(t, chunk)
	require.Equal(t, "chatcmpl-1", chunk.ID)
	require.Equal(t, "hi", chunk.Text)
	require.NotNil(t, chunk.Usage)
	require.NotNil(t, root)
	rawUsage, ok := root["usage"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, rawUsage["future_detail"])

	// Error frames surface the error while still returning the decoded object.
	_, root, err = transformer.ParseChunkWithMap([]byte(`{"error":{"message":"boom","type":"server_error"}}`))
	require.Error(t, err)
	require.NotNil(t, root)
	require.NotNil(t, root["error"])

	// Invalid JSON returns neither chunk nor map.
	_, root, err = transformer.ParseChunkWithMap([]byte("not json"))
	require.Error(t, err)
	require.Nil(t, root)
}
