package structured

import (
	"fmt"
	"regexp"
	"strings"
)

// checkMetaKeyword validates assertions and annotations that never produce
// child schemas. It reports whether the keyword is part of the supported set.
func checkMetaKeyword(path, key string, raw any) (bool, error) {
	invalid := func() error { return invalidKeyword(path, key) }
	switch key {
	case "$schema", "$id", "$comment", "title", "description":
		if _, ok := raw.(string); !ok {
			return true, invalid()
		}
	case "default", "examples", "deprecated", "readOnly", "writeOnly": // Annotations only.
	case "const": // Any JSON value; enforced against the instance later.
	case "enum":
		list, ok := raw.([]any)
		if !ok || len(list) == 0 {
			return true, invalid()
		}
	case "type":
		if !typeKeywordValid(raw) {
			return true, invalid()
		}
	case "required":
		list, ok := raw.([]any)
		if !ok {
			return true, invalid()
		}
		for _, item := range list {
			if _, ok := item.(string); !ok {
				return true, invalid()
			}
		}
	case "uniqueItems":
		if _, ok := raw.(bool); !ok {
			return true, invalid()
		}
	case "pattern":
		pattern, ok := raw.(string)
		if !ok {
			return true, invalid()
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return true, invalid()
		}
	default:
		return false, nil
	}
	return true, nil
}

func typeKeywordValid(raw any) bool {
	valid := func(v any) bool {
		t, ok := v.(string)
		return ok && (t == "null" || t == "object" || t == "array" || t == "string" || t == "boolean" || t == "number" || t == "integer")
	}
	if list, ok := raw.([]any); ok {
		if len(list) == 0 {
			return false
		}
		for _, item := range list {
			if !valid(item) {
				return false
			}
		}
		return true
	}
	return valid(raw)
}

// checkApplicatorKeyword validates keywords that introduce subschemas and
// records their reference edges. It reports whether the keyword is supported.
func (collector *definitionCollector) checkApplicatorKeyword(node *schemaNode, path, key string, raw any, depth int) (bool, error) {
	invalid := func() error { return invalidKeyword(path, key) }
	switch key {
	case "$ref":
		ref, ok := raw.(string)
		if !ok {
			return true, invalid()
		}
		if _, err := resolveReference(collector.root, ref); err != nil {
			return true, err
		}
		node.edges = append(node.edges, ref)
	case "$defs", "definitions", "properties":
		m, ok := raw.(map[string]any)
		if !ok {
			return true, invalid()
		}
		for name, value := range m {
			if err := collector.child(node, path+"/"+key+"/"+pointerPart(name), depth, value, false); err != nil {
				return true, err
			}
		}
	case "allOf", "anyOf", "oneOf":
		list, ok := raw.([]any)
		if !ok || len(list) == 0 {
			return true, invalid()
		}
		for i, value := range list {
			if err := collector.child(node, path+fmt.Sprintf("/%s/%d", key, i), depth, value, true); err != nil {
				return true, err
			}
		}
	case "not", "items":
		if err := collector.child(node, path+"/"+key, depth, raw, key == "not"); err != nil {
			return true, err
		}
	case "additionalProperties":
		if _, ok := raw.(bool); !ok {
			if err := collector.child(node, path+"/"+key, depth, raw, false); err != nil {
				return true, err
			}
		}
	default:
		return false, nil
	}
	return true, nil
}

// checkNumericBoundKeyword validates numeric bound assertions.
func checkNumericBoundKeyword(path, key string, raw any) (bool, error) {
	switch key {
	case "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf", "minLength", "maxLength", "minItems", "maxItems":
	default:
		return false, nil
	}
	n, ok := number(raw)
	if !ok {
		return true, invalidKeyword(path, key)
	}
	if key == "multipleOf" && n <= 0 {
		return true, invalidKeyword(path, key)
	}
	if strings.HasPrefix(key, "minL") || strings.HasPrefix(key, "maxL") || key == "minItems" || key == "maxItems" {
		if n < 0 || float64(int64(n)) != n {
			return true, invalidKeyword(path, key)
		}
	}
	return true, nil
}
