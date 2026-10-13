package openai

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestSchemaToMapPreservesRawJSON(t *testing.T) {
	t.Parallel()
	expected := map[string]any{"type": "object", "properties": map[string]any{"status": map[string]any{"type": "string"}}}
	raw, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		schema types.Schema
	}{
		{"map", expected},
		{"bytes", raw},
		{"raw message", json.RawMessage(raw)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := schemaToMap(test.schema)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("schema = %#v, want %#v", got, expected)
			}
			got["type"] = "changed"
			if expected["type"] != "object" {
				t.Fatal("conversion mutated caller schema")
			}
		})
	}
}

func TestSchemaToMapRejectsMalformedRawJSON(t *testing.T) {
	t.Parallel()
	if _, err := schemaToMap([]byte{'{'}); err == nil {
		t.Fatal("malformed raw schema accepted")
	}
}
