package wormhole

import (
	"encoding/json"
	"reflect"
	"testing"
)

type remediationSchemaLeaf struct {
	Value int `json:"value" tool:"required"`
}
type remediationSchemaCycle struct{ Next *remediationSchemaCycle }
type remediationEmbeddedA struct {
	Shared string
	Left   int
}
type remediationEmbeddedB struct {
	Shared int
	Right  bool
}
type remediationTagged struct {
	Shared int `json:"Shared"`
}

func TestRemediationC208RecursiveSchema(t *testing.T) {
	t.Parallel()
	type args struct {
		Rows [2][]**remediationSchemaLeaf `json:"rows"`
	}
	schema, err := SchemaFromStruct(&args{})
	if err != nil {
		t.Fatal(err)
	}
	rows := schema["properties"].(map[string]any)["rows"].(map[string]any)
	leaf := rows["items"].(map[string]any)["items"].(map[string]any)
	if leaf["type"] != "object" || !reflect.DeepEqual(leaf["required"], []string{"value"}) {
		t.Fatalf("nested schema: %#v", leaf)
	}
	if _, err := SchemaFromStruct(remediationSchemaCycle{}); err == nil {
		t.Fatal("recursive type accepted")
	}
	if _, err := SchemaFromStruct(nil); err == nil {
		t.Fatal("nil accepted")
	}
}

func TestRemediationC2I02EmbeddedJSONFields(t *testing.T) {
	t.Parallel()
	cases := []any{
		struct {
			remediationEmbeddedA
			remediationEmbeddedB
		}{},
		struct {
			remediationEmbeddedA
			remediationTagged
		}{},
		struct {
			*remediationEmbeddedA
			Shared bool
		}{remediationEmbeddedA: &remediationEmbeddedA{}},
		struct {
			remediationEmbeddedA `json:"nested"`
		}{},
	}
	for _, value := range cases {
		schema, err := SchemaFromStruct(value)
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var actual map[string]any
		if err := json.Unmarshal(b, &actual); err != nil {
			t.Fatal(err)
		}
		props := schema["properties"].(map[string]any)
		if len(props) != len(actual) {
			t.Fatalf("fields mismatch schema=%v json=%v", props, actual)
		}
		for key := range actual {
			if _, ok := props[key]; !ok {
				t.Fatalf("missing %s", key)
			}
		}
	}
}

func TestRemediationC2I01CompleteNumericTags(t *testing.T) {
	t.Parallel()
	for _, value := range []any{
		struct {
			N float64 `tool:"min=NaN"`
		}{},
		struct {
			N float64 `tool:"max=Inf"`
		}{},
		struct {
			N int `tool:"min=12tail"`
		}{},
		struct {
			S string `tool:"minLength=2.5"`
		}{},
	} {
		if _, err := SchemaFromStruct(value); err == nil {
			t.Fatalf("invalid tag accepted: %T", value)
		}
	}
	schema, err := SchemaFromStruct(struct {
		N float64 `tool:"min=-1.5e2;max=3.5"`
	}{})
	if err != nil {
		t.Fatal(err)
	}
	n := schema["properties"].(map[string]any)["N"].(map[string]any)
	if n["minimum"] != float64(-150) || n["maximum"] != 3.5 {
		t.Fatalf("constraints: %v", n)
	}
}
