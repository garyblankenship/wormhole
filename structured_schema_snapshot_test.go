package wormhole

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestStructuredSchemaSnapshotRetainsJSONObject(t *testing.T) {
	t.Parallel()
	type record struct {
		Value string `json:"value" tool:"required"`
	}
	schema := MustSchemaFromStruct(record{})
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	field := NewStringSchema().Build()
	object := NewObjectSchema().Property("value", &field).Required("value").Build()
	for _, sample := range []struct {
		name   string
		schema any
	}{
		{"generated", schema}, {"raw-bytes", data}, {"raw-message", json.RawMessage(data)}, {"fluent", &object},
	} {
		t.Run(sample.name, func(t *testing.T) {
			t.Parallel()
			builder := (&StructuredRequestBuilder{request: &types.StructuredRequest{}}).Schema(sample.schema)
			if builder.schemaErr != nil {
				t.Fatal(builder.schemaErr)
			}
			raw, err := json.Marshal(builder.request.Schema)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err = json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("snapshot is not a JSON object: %v", err)
			}
			if got["type"] != "object" || got["properties"] == nil {
				t.Fatalf("schema fields lost: %#v", got)
			}
		})
	}
}

func TestStructuredRawSchemaSnapshotIsDetached(t *testing.T) {
	t.Parallel()
	type record struct {
		Value string `json:"value"`
	}
	data, err := json.Marshal(MustSchemaFromStruct(record{}))
	if err != nil {
		t.Fatal(err)
	}
	expected := bytes.Clone(data)
	builder := (&StructuredRequestBuilder{request: &types.StructuredRequest{}}).Schema(data)
	data[0] = '!'
	if builder.schemaErr != nil {
		t.Fatal(builder.schemaErr)
	}
	cloned := cloneStructuredRequest(builder.request)
	builder.request.Schema.(json.RawMessage)[0] = '!'
	got, err := json.Marshal(cloned.Schema)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, expected) {
		t.Fatal("caller or builder mutation changed the execution snapshot")
	}
}

func TestStructuredInvalidRawSchemaFailsDuringSnapshot(t *testing.T) {
	t.Parallel()
	builder := (&StructuredRequestBuilder{request: &types.StructuredRequest{}}).Schema([]byte{0xff})
	if builder.schemaErr == nil || builder.request.Schema != nil {
		t.Fatal("invalid raw schema accepted")
	}
}
