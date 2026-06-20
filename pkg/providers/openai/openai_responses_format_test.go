package openai

import (
	"net/http"
	"testing"

	"encoding/json"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeResponsesFormat(t *testing.T) {
	t.Parallel()

	nested := map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "person",
			"strict": true,
			"schema": map[string]any{"type": "object"},
		},
	}
	result := normalizeResponsesFormat(nested)
	flat, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "json_schema", flat["type"])
	assert.Equal(t, "person", flat["name"])
	assert.Equal(t, true, flat["strict"])
	assert.NotNil(t, flat["schema"])
	_, hasNested := flat["json_schema"]
	assert.False(t, hasNested)

	objInput := map[string]string{"type": "json_object"}
	obj := normalizeResponsesFormat(objInput)
	require.Equal(t, objInput, obj)

	assert.Nil(t, normalizeResponsesFormat(nil))
	assert.Equal(t, "raw", normalizeResponsesFormat("raw"))
}

func TestBuildResponsesPayloadFlattensJSONSchema(t *testing.T) {
	t.Parallel()

	provider, _ := newOpenAITestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})

	payload := provider.buildResponsesPayload(&types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gpt-4o",
		},
		ResponseFormat: map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "person",
				"strict": true,
				"schema": map[string]any{"type": "object"},
			},
		},
	})

	text, ok := payload["text"].(map[string]any)
	require.True(t, ok)
	format, ok := text["format"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "json_schema", format["type"])
	assert.Equal(t, "person", format["name"])
	assert.Equal(t, true, format["strict"])
	assert.Equal(t, map[string]any{"type": "object"}, format["schema"])
	_, hasNested := format["json_schema"]
	assert.False(t, hasNested)
}
