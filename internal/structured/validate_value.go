package structured

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"unicode/utf8"
)

func (state *validationState) validateObject(root, s map[string]any, value map[string]any, path string, depth int) error {
	if required, ok := s["required"].([]any); ok {
		for _, item := range required {
			if err := state.spend(depth); err != nil {
				return err
			}
			if name, ok := item.(string); ok {
				if _, exists := value[name]; !exists {
					return validationFailure(path, "missing required property "+name)
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
			continue
		}
		if additional, ok := s["additionalProperties"].(bool); ok && !additional {
			return validationFailure(path, "unexpected property "+key)
		}
		if additional, ok := s["additionalProperties"].(map[string]any); ok {
			if err := state.validateValue(root, additional, item, path+"."+key, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func (state *validationState) validateArray(root, s map[string]any, value []any, path string, depth int) error {
	if n, ok := number(s["minItems"]); ok && float64(len(value)) < n {
		return validationFailure(path, "array has too few items")
	}
	if n, ok := number(s["maxItems"]); ok && float64(len(value)) > n {
		return validationFailure(path, "array has too many items")
	}
	items, _ := s["items"].(map[string]any)
	for index, item := range value {
		if err := state.spend(depth); err != nil {
			return err
		}
		if items != nil {
			if err := state.validateValue(root, items, item, fmt.Sprintf("%s[%d]", path, index), depth+1); err != nil {
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
					return validationFailure(path, "array items are not unique")
				}
			}
		}
	}
	return nil
}

func validateString(s map[string]any, value string, path string) error {
	size := float64(utf8.RuneCountInString(value))
	if n, ok := number(s["minLength"]); ok && size < n {
		return validationFailure(path, "string is too short")
	}
	if n, ok := number(s["maxLength"]); ok && size > n {
		return validationFailure(path, "string is too long")
	}
	if pattern, ok := s["pattern"].(string); ok {
		matched, err := regexp.MatchString(pattern, value)
		if err != nil {
			return validationFailure(path, "invalid schema pattern")
		}
		if !matched {
			return validationFailure(path, "string does not match pattern")
		}
	}
	return nil
}

func validateNumber(s map[string]any, v any, path string) error {
	n, ok := number(v)
	if !ok {
		return nil
	}
	if divisor, exists := s["multipleOf"]; exists {
		a, aok := new(big.Rat).SetString(fmt.Sprint(v))
		b, bok := new(big.Rat).SetString(fmt.Sprint(divisor))
		if !aok || !bok || b.Sign() <= 0 || !new(big.Rat).Quo(a, b).IsInt() {
			return validationFailure(path, "number is not a multipleOf")
		}
	}
	if bound, ok := number(s["minimum"]); ok && n < bound {
		return validationFailure(path, "number below minimum")
	}
	if bound, ok := number(s["maximum"]); ok && n > bound {
		return validationFailure(path, "number above maximum")
	}
	if bound, ok := number(s["exclusiveMinimum"]); ok && n <= bound {
		return validationFailure(path, "number below exclusive minimum")
	}
	if bound, ok := number(s["exclusiveMaximum"]); ok && n >= bound {
		return validationFailure(path, "number above exclusive maximum")
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
