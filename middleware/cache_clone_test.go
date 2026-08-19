package middleware

import (
	"encoding/json"
	"testing"
)

func TestCloneValueDeepCopiesNestedMetadata(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(map[string]string{"secret": "value"})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	original := map[string]any{
		"number": 7,
		"nested": map[string]any{
			"strings": []string{"one", "two"},
			"bytes":   []byte{1, 2},
			"raw":     json.RawMessage(raw),
		},
		"maps": []any{map[string]string{"key": "value"}},
	}

	clonedValue, err := cloneValue(original)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	cloned := clonedValue.(map[string]any)
	if _, ok := cloned["number"].(int); !ok {
		t.Fatalf("numeric type changed: %T", cloned["number"])
	}

	clonedNested := cloned["nested"].(map[string]any)
	clonedNested["strings"].([]string)[0] = "changed"
	clonedNested["bytes"].([]byte)[0] = 9
	clonedNested["raw"].(json.RawMessage)[0] = 'x'
	cloned["maps"].([]any)[0].(map[string]string)["key"] = "changed"

	originalNested := original["nested"].(map[string]any)
	if originalNested["strings"].([]string)[0] != "one" || originalNested["bytes"].([]byte)[0] != 1 || originalNested["raw"].(json.RawMessage)[0] == 'x' {
		t.Fatal("nested clone mutated original metadata")
	}
	if original["maps"].([]any)[0].(map[string]string)["key"] != "value" {
		t.Fatal("nested map clone mutated original metadata")
	}
}

func TestCloneValueTypedValuesAndErrors(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name string `json:"name"`
	}
	original := payload{Name: "value"}
	cloned, err := cloneValue(original)
	if err != nil || cloned != original {
		t.Fatalf("value clone = (%#v, %v)", cloned, err)
	}

	originalPointer := &payload{Name: "pointer"}
	clonedPointerValue, err := cloneValue(originalPointer)
	if err != nil {
		t.Fatalf("pointer clone: %v", err)
	}
	clonedPointer := clonedPointerValue.(*payload)
	if clonedPointer == originalPointer || *clonedPointer != *originalPointer {
		t.Fatalf("pointer was not independently cloned: %#v", clonedPointer)
	}

	var nilPointer *payload
	clonedNil, err := cloneValue(nilPointer)
	if err != nil || clonedNil != nilPointer {
		t.Fatalf("nil pointer clone = (%#v, %v)", clonedNil, err)
	}
	if _, err := cloneValue(func() {}); err == nil {
		t.Fatal("unsupported value clone succeeded")
	}
}
