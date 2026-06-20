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

	if _, ok := out["properties"]; !ok {
		t.Fatalf("expected properties in output, got %#v", out)
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
