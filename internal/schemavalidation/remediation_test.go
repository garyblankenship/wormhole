package schemavalidation

import (
	"encoding/json"
	"math"
	"testing"
)

func TestRemediationC201C204C209C2I02(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "required": []string{"count"}, "properties": map[string]any{
		"count":  map[string]any{"type": "integer", "enum": []any{1}},
		"label":  map[string]any{"type": "string", "enum": []string{"ok"}},
		"values": map[string]any{"type": "array"},
	}}
	for _, decode := range []bool{false, true} {
		t.Run(map[bool]string{false: "generated", true: "decoded"}[decode], func(t *testing.T) {
			t.Parallel()
			s := schema
			if decode {
				b, err := json.Marshal(schema)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(b, &s); err != nil {
					t.Fatal(err)
				}
			}
			if err := ValidateAgainstSchema(map[string]any{"count": float64(1), "label": "ok", "values": []any{1, "two", nil}}, s); err != nil {
				t.Fatal(err)
			}
			for _, data := range []map[string]any{
				{}, {"count": 1.5}, {"count": math.Inf(1)}, {"count": math.NaN()}, {"count": "1"}, {"count": 2}, {"count": 1, "label": 1},
			} {
				if err := ValidateAgainstSchema(data, s); err == nil {
					t.Fatalf("accepted %#v", data)
				}
			}
		})
	}
}
