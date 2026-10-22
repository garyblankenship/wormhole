package structured

import (
	"fmt"

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

func validationFailure(path, message string) error {
	return fmt.Errorf("%s: %s", path, message)
}

func (state *validationState) validateValue(root, s map[string]any, v any, path string, depth int) error {
	if err := state.spend(depth); err != nil {
		return err
	}
	if ref, ok := s["$ref"].(string); ok {
		mapped, err := resolveReference(root, ref)
		if err != nil {
			return err
		}
		if err := state.validateValue(root, mapped, v, path, depth+1); err != nil {
			return err
		}
	}
	if err := validateTypeKeyword(s, v, path); err != nil {
		return err
	}
	if value, ok := s["const"]; ok && !jsonEqual(value, v) {
		return validationFailure(path, "value does not match const")
	}
	if err := state.validateEnum(s, v, path, depth); err != nil {
		return err
	}
	if err := state.validateCombinators(root, s, v, path, depth); err != nil {
		return err
	}
	return state.validateTypedValue(root, s, v, path, depth)
}

// validateTypeKeyword enforces the optional "type" assertion.
func validateTypeKeyword(s map[string]any, v any, path string) error {
	typespec, ok := s["type"]
	if !ok {
		return nil
	}
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
		return validationFailure(path, "value does not match schema type")
	}
	return nil
}

// validateEnum enforces the "enum" assertion, spending work for every candidate
// so the shared budget sees the full enumeration.
func (state *validationState) validateEnum(s map[string]any, v any, path string, depth int) error {
	values, ok := s["enum"].([]any)
	if !ok {
		return nil
	}
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
		return validationFailure(path, "value is outside enum")
	}
	return nil
}

// validateCombinators enforces allOf, anyOf, oneOf, and not.
func (state *validationState) validateCombinators(root, s map[string]any, v any, path string, depth int) error {
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		branches, ok := s[key].([]any)
		if !ok {
			continue
		}
		matches := 0
		for _, branch := range branches {
			if m, ok := branch.(map[string]any); ok && state.validateValue(root, m, v, path, depth+1) == nil {
				matches++
			}
		}
		if key == "allOf" && matches != len(branches) || key == "anyOf" && matches == 0 || key == "oneOf" && matches != 1 {
			return validationFailure(path, "value does not satisfy "+key)
		}
	}
	if neg, ok := s["not"].(map[string]any); ok && state.validateValue(root, neg, v, path, depth+1) == nil {
		return validationFailure(path, "value satisfies forbidden schema")
	}
	return nil
}

// validateTypedValue dispatches to the assertion set for the value's JSON kind.
func (state *validationState) validateTypedValue(root, s map[string]any, v any, path string, depth int) error {
	switch value := v.(type) {
	case map[string]any:
		return state.validateObject(root, s, value, path, depth)
	case []any:
		return state.validateArray(root, s, value, path, depth)
	case string:
		return validateString(s, value, path)
	default:
		return validateNumber(s, v, path)
	}
}
