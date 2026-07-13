package gemini

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// A *types.ObjectSchema must convert with full fidelity (properties + required),
// not be flattened to {"type":"object"} by the SchemaInterface branch.
// Regression guard for the dispatch-order bug.
func TestSchemaToMapObjectFullFidelity(t *testing.T) {
	t.Parallel()
	g := &Gemini{}

	schema := &types.ObjectSchema{
		Properties: map[string]types.SchemaInterface{
			"city": &types.StringSchema{},
		},
		Required: []string{"city"},
	}

	out := g.schemaToMap(schema)

	if out["type"] != "object" {
		t.Fatalf("expected type=object in output, got %#v", out)
	}
	props, ok := out["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties in output, got %#v", out)
	}
	city, ok := props["city"].(map[string]any)
	if !ok || city["type"] != "string" {
		t.Fatalf("expected city property with type=string, got %#v", props["city"])
	}
	req, ok := out["required"]
	if !ok {
		t.Fatalf("expected required in output, got %#v", out)
	}
	reqSlice, ok := req.([]string)
	if !ok || len(reqSlice) != 1 || reqSlice[0] != "city" {
		t.Fatalf("required = %#v, want [city]", req)
	}
}

func TestNormalizeSchemaMap(t *testing.T) {
	t.Parallel()
	// [T,"null"] -> type:T + nullable:true
	m := map[string]any{"type": []any{"string", "null"}}
	normalized := normalizeSchemaMap(m)
	if _, mutated := m["nullable"]; mutated {
		t.Fatalf("normalization mutated caller schema: %#v", m)
	}
	m = normalized
	if m["type"] != "string" || m["nullable"] != true {
		t.Fatalf("nullable case: got %#v", m)
	}
	// multi-type union -> anyOf, type removed
	m2 := map[string]any{"type": []any{"string", "number"}}
	m2 = normalizeSchemaMap(m2)
	if _, ok := m2["type"]; ok {
		t.Fatalf("multi-type: type should be removed, got %#v", m2)
	}
	if branches, ok := m2["anyOf"].([]any); !ok || len(branches) != 2 {
		t.Fatalf("multi-type: anyOf wrong, got %#v", m2)
	}
	// recursive: nested in properties + array items
	m3 := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": []any{"string", "null"}},
			"tags": map[string]any{"type": "array", "items": map[string]any{"type": []any{"string", "null"}}},
		},
	}
	m3 = normalizeSchemaMap(m3)
	name := m3["properties"].(map[string]any)["name"].(map[string]any)
	if name["type"] != "string" || name["nullable"] != true {
		t.Fatalf("nested property not normalized: %#v", name)
	}
	items := m3["properties"].(map[string]any)["tags"].(map[string]any)["items"].(map[string]any)
	if items["type"] != "string" || items["nullable"] != true {
		t.Fatalf("nested items not normalized: %#v", items)
	}
	// no regression: plain object/string untouched
	m4 := map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}}
	m4 = normalizeSchemaMap(m4)
	if m4["type"] != "object" {
		t.Fatalf("plain object changed: %#v", m4)
	}
}
