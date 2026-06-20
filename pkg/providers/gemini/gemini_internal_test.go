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
		"type":      "string",
		"minLength": minLength,
		"maxLength": maxLength,
		"pattern":   "^[a-z]+$",
	}, nameResult)

	scoreResult := map[string]any{}
	provider.numberSchemaToMap(schema.Properties["score"].(*types.NumberSchema), scoreResult)
	assert.Equal(t, map[string]any{
		"type":    "number",
		"minimum": minimum,
		"maximum": maximum,
	}, scoreResult)

	assert.Equal(t, map[string]any{
		"type":      "string",
		"minLength": minLength,
		"maxLength": maxLength,
		"pattern":   "^[a-z]+$",
	}, properties["name"])
	assert.Equal(t, map[string]any{
		"type":    "number",
		"minimum": minimum,
		"maximum": maximum,
	}, properties["score"])
	assert.Equal(t, map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "string",
			"enum": []any{"new", "known"},
		},
	}, properties["tags"])

	assert.Equal(t, map[string]any{
		"type": "string",
		"enum": []any{"new", "known"},
	}, provider.schemaTypeToMap(&types.EnumSchema{Enum: []any{"new", "known"}}))
	assert.Equal(t, map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "string",
			"enum": []any{"new", "known"},
		},
	}, provider.schemaTypeToMap(&types.ArraySchema{
		Items: &types.EnumSchema{
			BaseSchema: types.BaseSchema{Type: "string"},
			Enum:       []any{"new", "known"},
		},
	}))
}

func TestTransformTextResponse_SyntheticToolCallIDs(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})

	// Failure mode under test: the same function is called twice in one
	// turn. With name-as-ID the two IDs collided; synthetic IDs must differ.
	resp := &geminiTextResponse{
		Candidates: []candidate{
			{
				Content: content{
					Parts: []part{
						{FunctionCall: &functionCall{Name: "get_weather", Args: map[string]any{"city": "NYC"}}},
						{FunctionCall: &functionCall{Name: "get_weather", Args: map[string]any{"city": "LA"}}},
					},
				},
			},
		},
	}

	out, err := provider.transformTextResponse(resp)
	assert.NoError(t, err)
	assert.Len(t, out.ToolCalls, 2)

	assert.Equal(t, "get_weather", out.ToolCalls[0].Name)
	assert.Equal(t, "get_weather", out.ToolCalls[1].Name)
	assert.Equal(t, "gemini-call-0-get_weather", out.ToolCalls[0].ID)
	assert.Equal(t, "gemini-call-1-get_weather", out.ToolCalls[1].ID)
	assert.NotEqual(t, out.ToolCalls[0].ID, out.ToolCalls[1].ID)
}

func TestProcessStreamCandidate_SyntheticToolCallIDs(t *testing.T) {
	t.Parallel()

	provider := New("test-key", types.ProviderConfig{})

	cand := candidate{
		Content: content{
			Parts: []part{
				{FunctionCall: &functionCall{Name: "lookup", Args: map[string]any{"q": "a"}}},
				{FunctionCall: &functionCall{Name: "lookup", Args: map[string]any{"q": "b"}}},
			},
		},
	}

	chunks := provider.processStreamCandidate(cand)

	var ids []string
	for _, c := range chunks {
		if c.ToolCall != nil {
			ids = append(ids, c.ToolCall.ID)
			assert.Equal(t, "lookup", c.ToolCall.Name)
		}
	}
	assert.Equal(t, []string{"gemini-call-0-lookup", "gemini-call-1-lookup"}, ids)
}
