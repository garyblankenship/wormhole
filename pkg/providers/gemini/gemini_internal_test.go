package gemini

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestTransformToolChoiceActual(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})

	assert.Nil(t, provider.transformToolChoice(nil))
	assert.Equal(t, map[string]any{
		"functionCallingConfig": map[string]any{"mode": "AUTO"},
	}, provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeAuto}))
	assert.Equal(t, map[string]any{
		"functionCallingConfig": map[string]any{"mode": "NONE"},
	}, provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeNone}))
	assert.Equal(t, map[string]any{
		"functionCallingConfig": map[string]any{"mode": "ANY"},
	}, provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeAny}))
	assert.Equal(t, map[string]any{
		"functionCallingConfig": map[string]any{
			"mode":                 "ANY",
			"allowedFunctionNames": []string{"lookup"},
		},
	}, provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: "lookup"}))
	assert.Nil(t, provider.transformToolChoice(&types.ToolChoice{Type: types.ToolChoiceType("unknown")}))
}

func TestSchemaToMapActualSchemaTypes(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})
	minLength := 2
	maxLength := 12
	minimum := 1.5
	maximum := 99.5

	schema := &types.ObjectSchema{
		BaseSchema: types.BaseSchema{Type: "object"},
		Required:   []string{"name"},
		Properties: map[string]types.SchemaInterface{
			"name": &types.StringSchema{
				BaseSchema: types.BaseSchema{Type: "string", Description: "display name"},
				MinLength:  &minLength,
				MaxLength:  &maxLength,
				Pattern:    "^[a-z]+$",
			},
			"score": &types.NumberSchema{
				BaseSchema: types.BaseSchema{Type: "number"},
				Minimum:    &minimum,
				Maximum:    &maximum,
			},
			"tags": &types.ArraySchema{
				BaseSchema: types.BaseSchema{Type: "array"},
				Items: &types.EnumSchema{
					BaseSchema: types.BaseSchema{Type: "string"},
					Enum:       []any{"new", "known"},
				},
			},
		},
	}

	assert.Equal(t, map[string]any{
		"type":        "string",
		"description": "display name",
	}, provider.schemaInterfaceToMap(schema.Properties["name"]))

	result := map[string]any{}
	provider.objectSchemaToMap(schema, result)
	assert.Equal(t, []string{"name"}, result["required"])
	properties := result["properties"].(map[string]any)

	nameResult := map[string]any{}
	provider.stringSchemaToMap(schema.Properties["name"].(*types.StringSchema), nameResult)
	assert.Equal(t, map[string]any{
		"minLength": minLength,
		"maxLength": maxLength,
		"pattern":   "^[a-z]+$",
	}, nameResult)

	scoreResult := map[string]any{}
	provider.numberSchemaToMap(schema.Properties["score"].(*types.NumberSchema), scoreResult)
	assert.Equal(t, map[string]any{
		"minimum": minimum,
		"maximum": maximum,
	}, scoreResult)

	assert.Equal(t, map[string]any{
		"type":        "string",
		"description": "display name",
	}, properties["name"])
	assert.Equal(t, map[string]any{"type": "number"}, properties["score"])
	assert.Equal(t, map[string]any{"type": "array"}, properties["tags"])

	assert.Equal(t, map[string]any{
		"enum": []any{"new", "known"},
	}, provider.schemaTypeToMap(&types.EnumSchema{Enum: []any{"new", "known"}}))
	assert.Equal(t, map[string]any{
		"items": map[string]any{"type": "string"},
	}, provider.schemaTypeToMap(&types.ArraySchema{
		Items: &types.EnumSchema{
			BaseSchema: types.BaseSchema{Type: "string"},
			Enum:       []any{"new", "known"},
		},
	}))
}
