package wormhole

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/garyblankenship/wormhole/v3/internal/schemavalidation"
	"github.com/garyblankenship/wormhole/v3/types"
)

func TestAuditRemediationBatchNilEntries(t *testing.T) {
	t.Parallel()
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprintf("mixed=%t", mixed), func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			client := newBatchTestClient(func(_ context.Context, request types.TextRequest) (*types.TextResponse, error) {
				calls.Add(1)
				return &types.TextResponse{Model: request.Model}, nil
			})
			t.Cleanup(func() { _ = client.Close() })
			batch := client.Batch().Add(nil)
			if mixed {
				batch.AddAll(client.Text().Model("first").Prompt("hello"), nil, client.Text().Model("second").Prompt("hello"))
			}
			results := batch.Execute(t.Context())
			if len(results) != batch.Count() {
				t.Fatalf("result count = %d, want %d", len(results), batch.Count())
			}
			for i, result := range results {
				if result.Index != i {
					t.Fatalf("result %d has index %d", i, result.Index)
				}
				if i%2 == 0 {
					if !errors.Is(result.Error, types.ErrInvalidRequest) || !strings.Contains(result.Error.Error(), fmt.Sprintf("request %d", i)) || result.Response != nil {
						t.Fatalf("nil entry %d result = %#v", i, result)
					}
				} else if result.Error != nil || result.Response == nil || result.Response.Model != []string{"first", "second"}[i/2] {
					t.Fatalf("valid entry %d result = %#v", i, result)
				}
			}
			wantCalls := int32(0)
			if mixed {
				wantCalls = 2
			}
			if calls.Load() != wantCalls {
				t.Fatalf("provider calls = %d, want %d", calls.Load(), wantCalls)
			}
		})
	}
}

func TestAuditRemediationBuilderOptions(t *testing.T) {
	t.Parallel()
	want := types.ToolChoice{Type: types.ToolChoiceTypeAuto}
	for _, choice := range []any{"auto", types.ToolChoiceTypeAuto, want, &want} {
		builder := (&TextRequestBuilder{request: &types.TextRequest{}}).ToolChoice(choice)
		if builder.request.ToolChoice == nil || *builder.request.ToolChoice != want {
			t.Fatalf("ToolChoice(%T) = %#v", choice, builder.request.ToolChoice)
		}
	}
	choice := types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: "lookup"}
	builder := (&TextRequestBuilder{request: &types.TextRequest{}}).ToolChoice(&choice)
	choice.ToolName = "changed"
	if builder.request.ToolChoice.ToolName != "lookup" {
		t.Fatal("caller mutation changed pointer tool choice")
	}
	builder.ToolChoice(choice)
	choice.ToolName = "changed again"
	if builder.request.ToolChoice.ToolName != "changed" {
		t.Fatal("caller mutation changed value tool choice")
	}
	builder.ToolChoice(42)
	if builder.request.ToolChoice.ToolName != "changed" {
		t.Fatal("unsupported type changed existing tool choice")
	}
	builder.ToolChoice((*types.ToolChoice)(nil))
	if builder.request.ToolChoice != nil {
		t.Fatal("typed nil did not clear tool choice")
	}
	enabled, thoughts := true, true
	builder.Reasoning(types.Reasoning{Enabled: &enabled, IncludeThoughts: &thoughts})
	enabled, thoughts = false, false
	if !*builder.request.Reasoning.Enabled || !*builder.request.Reasoning.IncludeThoughts {
		t.Fatal("caller mutation changed reasoning flags")
	}
}

type auditEnumInt int16
type auditEnumString string
type auditEnumFloat float64
type auditEnumFloat32 float32
type auditEnumBool bool

func TestAuditRemediationTypedEnums(t *testing.T) {
	t.Parallel()
	schema, err := SchemaFromStruct(struct {
		Optional string          `json:"optional" tool:"enum=active|required,other"`
		Required string          `json:"required" tool:"enum=optional,required;required"`
		Before   string          `json:"before" tool:"required,enum=yes|no;minLength=2"`
		Count    int8            `json:"count" tool:"enum=-1,2"`
		Unsigned uint8           `json:"unsigned" tool:"enum=0|255"`
		Fraction float32         `json:"fraction" tool:"enum=1.25|2.5"`
		Switch   bool            `json:"switch" tool:"enum=true,false"`
		NamedInt auditEnumInt    `json:"namedInt" tool:"enum=-3|4"`
		NamedStr auditEnumString `json:"namedStr" tool:"enum=hello|world"`
		NamedNum auditEnumFloat  `json:"namedNum" tool:"enum=1.5|2.5"`
		NamedBit auditEnumBool   `json:"namedBit" tool:"enum=true|false"`
		Pattern  string          `json:"pattern" tool:"pattern=^[a,b]+$;minLength=1"`
	}{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema["required"], []string{"required", "before"}) {
		t.Fatalf("required = %#v", schema["required"])
	}
	props := schema["properties"].(map[string]any)
	if got := props["pattern"].(map[string]any)["pattern"]; got != "^[a,b]+$" {
		t.Fatalf("semicolon-delimited pattern = %#v", got)
	}
	wantEnums := map[string]any{
		"optional": []string{"active", "required", "other"},
		"count":    []any{int64(-1), int64(2)}, "unsigned": []any{uint64(0), uint64(255)},
		"fraction": []any{float32(1.25), float32(2.5)}, "switch": []any{true, false},
		"namedInt": []any{int64(-3), int64(4)}, "namedStr": []string{"hello", "world"},
		"namedNum": []any{1.5, 2.5}, "namedBit": []any{true, false},
	}
	for name, want := range wantEnums {
		if got := props[name].(map[string]any)["enum"]; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s enum = %#v, want %#v", name, got, want)
		}
	}
	input := map[string]any{"required": "required", "before": "yes", "count": -1, "unsigned": 255,
		"fraction": 1.25, "switch": true, "namedInt": -3, "namedStr": "hello", "namedNum": 1.5, "namedBit": false}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := schemavalidation.ValidateAgainstSchema(decoded, schema); err != nil {
		t.Fatalf("matching JSON input rejected: %v", err)
	}
	decoded["count"] = float64(3)
	if err := schemavalidation.ValidateAgainstSchema(decoded, schema); err == nil {
		t.Fatal("non-enum JSON value accepted")
	}
}

func TestAuditRemediationFloat32EnumJSON(t *testing.T) {
	t.Parallel()
	schema, err := SchemaFromStruct(struct {
		Plain float32          `json:"plain" tool:"enum=0.1"`
		Named auditEnumFloat32 `json:"named" tool:"enum=0.1"`
	}{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	var wireSchema map[string]any
	if err := json.Unmarshal(encoded, &wireSchema); err != nil {
		t.Fatal(err)
	}
	properties := wireSchema["properties"].(map[string]any)
	for _, name := range []string{"plain", "named"} {
		if got := properties[name].(map[string]any)["enum"].([]any)[0]; got != float64(0.1) {
			t.Fatalf("%s serialized enum = %#v, want 0.1", name, got)
		}
	}
	input, err := json.Marshal(map[string]any{"plain": 0.1, "named": 0.1})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(input, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := schemavalidation.ValidateAgainstSchema(decoded, schema); err != nil {
		t.Fatalf("matching JSON input rejected: %v", err)
	}
	if err := schemavalidation.ValidateAgainstSchema(decoded, wireSchema); err != nil {
		t.Fatalf("serialized schema rejected matching JSON input: %v", err)
	}
}

func TestAuditRemediationRejectInvalidEnums(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		value any
		enum  string
	}{
		{"integer malformed", int(0), "1.5"},
		{"integer range", int8(0), "128"},
		{"unsigned negative", uint8(0), "-1"},
		{"unsigned range", uint8(0), "256"},
		{"float range", float32(0), "1e40"},
		{"float malformed", float64(0), "1.2x"},
		{"float nan", float64(0), "NaN"},
		{"float infinity", float64(0), "Inf"},
		{"boolean malformed", false, "maybe"},
		{"named range", auditEnumInt(0), "32768"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			typ := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: reflect.TypeOf(test.value), Tag: reflect.StructTag(fmt.Sprintf("tool:%q", "enum="+test.enum))}})
			if _, err := SchemaFromStruct(reflect.New(typ).Interface()); err == nil {
				t.Fatalf("invalid enum %q accepted for %s", test.enum, reflect.TypeOf(test.value))
			}
		})
	}
}
