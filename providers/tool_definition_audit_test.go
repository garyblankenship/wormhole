package providers

import (
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestAuditTransformToolNestedAndFlatPrecedence(t *testing.T) {
	t.Parallel()
	b := NewRequestBuilder()
	tool := types.Tool{Function: &types.ToolFunction{Name: "nested", Description: "nested description", Parameters: map[string]any{"type": "object"}}}
	got := b.TransformTool(tool)["function"].(map[string]any)
	if got["name"] != "nested" || got["description"] != "nested description" || got["parameters"].(map[string]any)["type"] != "object" {
		t.Fatal(got)
	}
	got["parameters"].(map[string]any)["type"] = "changed"
	if tool.Function.Parameters["type"] != "object" {
		t.Fatal("caller schema mutated")
	}
	tool.Name, tool.Description = "flat", "flat description"
	tool.InputSchema = map[string]any{}
	got = b.TransformTool(tool)["function"].(map[string]any)
	if got["name"] != "flat" || got["description"] != "flat description" || len(got["parameters"].(map[string]any)) != 0 {
		t.Fatal(got)
	}
}
