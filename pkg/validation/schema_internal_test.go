package validation

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSchemaInferenceAndTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{
			name: "object inferred from properties",
			schema: map[string]any{
				"properties": map[string]any{"name": map[string]any{"type": "string"}},
			},
			want: "object",
		},
		{
			name: "array inferred from items",
			schema: map[string]any{
				"items": map[string]any{"type": "string"},
			},
			want: "array",
		},
		{
			name: "enum inferred from enum",
			schema: map[string]any{
				"enum": []any{"a", "b"},
			},
			want: "enum",
		},
		{
			name:   "unknown type falls back to object",
			schema: map[string]any{"type": "mystery"},
			want:   "object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parseSchema(tt.schema)
			require.NoError(t, err)
			assert.Equal(t, tt.want, parsed.GetType())
		})
	}
}

func TestParseSchemaErrorsAndFallbacks(t *testing.T) {
	t.Parallel()
	_, err := parseSchema(nil)
	require.Error(t, err)

	parsed, err := parseSchema(map[string]any{
		"type":  "object",
		"items": "ignored",
		"properties": map[string]any{
			"skip": "not a schema",
			"bad": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
	})
	require.NoError(t, err)
	objectSchema, ok := parsed.(*types.ObjectSchema)
	require.True(t, ok)
	assert.NotContains(t, objectSchema.Properties, "skip")
	assert.Contains(t, objectSchema.Properties, "bad")

	array, err := parseSchema(map[string]any{
		"type":  "array",
		"items": "string",
	})
	require.NoError(t, err)
	arraySchema, ok := array.(*types.ArraySchema)
	require.True(t, ok)
	assert.Equal(t, "string", arraySchema.Items.GetType())
}

func TestParseScalarSchemas(t *testing.T) {
	t.Parallel()
	stringSchema := parseStringSchema(map[string]any{
		"type":        "string",
		"description": "name",
		"minLength":   int64(2),
		"maxLength":   int32(5),
		"pattern":     "^[A-Z]",
	})
	assert.Equal(t, "name", stringSchema.Description)
	require.NotNil(t, stringSchema.MinLength)
	require.NotNil(t, stringSchema.MaxLength)
	assert.Equal(t, 2, *stringSchema.MinLength)
	assert.Equal(t, 5, *stringSchema.MaxLength)

	numberSchema := parseNumberSchema(map[string]any{
		"minimum": int64(1),
		"maximum": float32(2.5),
	})
	require.NotNil(t, numberSchema.Minimum)
	require.NotNil(t, numberSchema.Maximum)
	assert.Equal(t, 1.0, *numberSchema.Minimum)
	assert.Equal(t, 2.5, *numberSchema.Maximum)

	booleanSchema := parseBooleanSchema(map[string]any{"description": "enabled"})
	assert.Equal(t, "enabled", booleanSchema.Description)

	enumSchema := parseEnumSchema(map[string]any{"enum": []any{1, 2}})
	assert.Equal(t, []any{1, 2}, enumSchema.Enum)
	assert.Nil(t, getIntPtr(map[string]any{"bad": "x"}, "bad"))
	assert.Nil(t, getFloat64Ptr(map[string]any{"bad": "x"}, "bad"))
}
