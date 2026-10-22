package structured

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"unicode/utf8"

	"github.com/garyblankenship/wormhole/v3/types"
)

// Validate applies interface schemas or JSON Schema constraints to final data.
func Validate(schema types.Schema, data any) error {
	if validator, ok := schema.(types.SchemaInterface); ok {
		return validator.Validate(data)
	}
	definition, err := schemaMap(schema)
	if err != nil {
		return err
	}
	state := &validationState{}
	err = state.validateValue(definition, definition, data, "$", 0)
	if state.err != nil {
		return state.err
	}
	return err
}

type validationState struct {
	work int
	err  error
}

func (state *validationState) spend(depth int) error {
	state.work++
	if state.err == nil && (depth > maxSchemaDepth || state.work > maxSchemaWork) {
		state.err = fmt.Errorf("schema validation complexity limit exceeded")
	}
	return state.err
}

func (state *validationState) validateValue(root, s map[string]any, v any, path string, depth int) error {
	if err := state.spend(depth); err != nil {
		return err
	}
	fail := func(message string) error { return fmt.Errorf("%s: %s", path, message) }
	if ref, ok := s["$ref"].(string); ok {
		mapped, err := resolveReference(root, ref)
		if err != nil {
			return err
		}
		if err := state.validateValue(root, mapped, v, path, depth+1); err != nil {
			return err
		}
	}
	if typespec, ok := s["type"]; ok {
		match := false
		switch ts := typespec.(type) {
		case string:
			match = matchesType(ts, v)
		case []any:
			for _, typ := range ts {
				if text, ok := typ.(string); ok && matchesType(text, v) {
					match = true
				}
			}
		}
		if !match {
			return fail("value does not match schema type")
		}
	}
	if value, ok := s["const"]; ok && !jsonEqual(value, v) {
		return fail("value does not match const")
	}
	if values, ok := s["enum"].([]any); ok {
		found := false
		for _, item := range values {
			if err := state.spend(depth); err != nil {
				return err
			}
			if jsonEqual(item, v) {
				found = true
			}
		}
		if !found {
			return fail("value is outside enum")
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if branches, ok := s[key].([]any); ok {
			matches := 0
			for _, branch := range branches {
				if m, ok := branch.(map[string]any); ok && state.validateValue(root, m, v, path, depth+1) == nil {
					matches++
				}
			}
			if key == "allOf" && matches != len(branches) || key == "anyOf" && matches == 0 || key == "oneOf" && matches != 1 {
				return fail("value does not satisfy " + key)
			}
		}
	}
	if neg, ok := s["not"].(map[string]any); ok && state.validateValue(root, neg, v, path, depth+1) == nil {
		return fail("value satisfies forbidden schema")
	}
	switch value := v.(type) {
	case map[string]any:
		if required, ok := s["required"].([]any); ok {
			for _, item := range required {
				if err := state.spend(depth); err != nil {
					return err
				}
				if name, ok := item.(string); ok {
					if _, exists := value[name]; !exists {
						return fail("missing required property " + name)
					}
				}
			}
		}
		properties, _ := s["properties"].(map[string]any)
		for key, item := range value {
			if err := state.spend(depth); err != nil {
				return err
			}
			if property, exists := properties[key]; exists {
				if m, ok := property.(map[string]any); ok {
					if err := state.validateValue(root, m, item, path+"."+key, depth+1); err != nil {
						return err
					}
				}
			} else if additional, ok := s["additionalProperties"].(bool); ok && !additional {
				return fail("unexpected property " + key)
			} else if additional, ok := s["additionalProperties"].(map[string]any); ok {
				if err := state.validateValue(root, additional, item, path+"."+key, depth+1); err != nil {
					return err
				}
			}
		}
	case []any:
		if n, ok := number(s["minItems"]); ok && float64(len(value)) < n {
			return fail("array has too few items")
		}
		if n, ok := number(s["maxItems"]); ok && float64(len(value)) > n {
			return fail("array has too many items")
		}
		for index, item := range value {
			if err := state.spend(depth); err != nil {
				return err
			}
			if m, ok := s["items"].(map[string]any); ok {
				if err := state.validateValue(root, m, item, fmt.Sprintf("%s[%d]", path, index), depth+1); err != nil {
					return err
				}
			}
		}
		if unique, _ := s["uniqueItems"].(bool); unique {
			for i := range value {
				for j := 0; j < i; j++ {
					if err := state.spend(depth); err != nil {
						return err
					}
					if jsonEqual(value[i], value[j]) {
						return fail("array items are not unique")
					}
				}
			}
		}
	case string:
		size := float64(utf8.RuneCountInString(value))
		if n, ok := number(s["minLength"]); ok && size < n {
			return fail("string is too short")
		}
		if n, ok := number(s["maxLength"]); ok && size > n {
			return fail("string is too long")
		}
		if pattern, ok := s["pattern"].(string); ok {
			matched, err := regexp.MatchString(pattern, value)
			if err != nil {
				return fail("invalid schema pattern")
			}
			if !matched {
				return fail("string does not match pattern")
			}
		}
	default:
		if n, ok := number(value); ok {
			if divisor, exists := s["multipleOf"]; exists {
				a, aok := new(big.Rat).SetString(fmt.Sprint(value))
				b, bok := new(big.Rat).SetString(fmt.Sprint(divisor))
				if !aok || !bok || b.Sign() <= 0 || !new(big.Rat).Quo(a, b).IsInt() {
					return fail("number is not a multipleOf")
				}
			}
			if bound, ok := number(s["minimum"]); ok && n < bound {
				return fail("number below minimum")
			}
			if bound, ok := number(s["maximum"]); ok && n > bound {
				return fail("number above maximum")
			}
			if bound, ok := number(s["exclusiveMinimum"]); ok && n <= bound {
				return fail("number below exclusive minimum")
			}
			if bound, ok := number(s["exclusiveMaximum"]); ok && n >= bound {
				return fail("number above exclusive maximum")
			}
		}
	}
	return nil
}
func matchesType(t string, v any) bool {
	switch t {
	case "null":
		return v == nil
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "number":
		_, ok := number(v)
		return ok
	case "integer":
		n, ok := number(v)
		return ok && math.Trunc(n) == n
	}
	return false
}
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, e := n.Float64()
		return f, e == nil
	}
	return 0, false
}
func jsonEqual(a, b any) bool {
	if av, ok := number(a); ok {
		bv, ok := number(b)
		return ok && av == bv
	}
	return reflect.DeepEqual(a, b)
}
