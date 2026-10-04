package gemini

import (
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

// C4-04: recursive concrete schemas retain descriptions at every level.
func TestRemediationC404SchemaDescriptions(t *testing.T) {
	t.Parallel()
	stringSchema := &types.StringSchema{BaseSchema: types.BaseSchema{Type: "string", Description: "string description"}}
	array := &types.ArraySchema{BaseSchema: types.BaseSchema{Type: "array", Description: "array description"}, Items: stringSchema}
	schema := &types.ObjectSchema{BaseSchema: types.BaseSchema{Type: "object", Description: "object description"}, Properties: map[string]types.SchemaInterface{
		"array": array, "number": &types.NumberSchema{BaseSchema: types.BaseSchema{Type: "number", Description: "number description"}}, "enum": &types.EnumSchema{BaseSchema: types.BaseSchema{Type: "string", Description: "enum description"}, Enum: []any{"a"}},
	}}
	output := (&Gemini{}).schemaToMap(schema)
	if output["description"] != "object description" {
		t.Fatal(output)
	}
	props := output["properties"].(map[string]any)
	for _, key := range []string{"array", "number", "enum"} {
		if props[key].(map[string]any)["description"] != key+" description" {
			t.Fatalf("%s: %#v", key, props[key])
		}
	}
	if props["array"].(map[string]any)["items"].(map[string]any)["description"] != "string description" {
		t.Fatal(props)
	}
}
