package wormhole

import (
	"fmt"
	"reflect"
)

// SchemaFromStruct generates a JSON Schema using encoding/json field selection
// and tool/desc tags. Recursive Go types return an error rather than recursing indefinitely.
func SchemaFromStruct(v any) (map[string]any, error) {
	t := reflect.TypeOf(v)
	if t == nil {
		return nil, fmt.Errorf("expected struct, got nil")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %s", t.Kind())
	}
	return schemaForType(t, make(map[reflect.Type]bool))
}

func schemaForType(t reflect.Type, visiting map[reflect.Type]bool) (map[string]any, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if visiting[t] {
		return nil, fmt.Errorf("recursive schema type %s", t)
	}
	visiting[t] = true
	defer delete(visiting, t)
	schema := map[string]any{"type": goTypeToJSONType(t)}
	switch t.Kind() {
	case reflect.Array, reflect.Slice:
		items, err := schemaForType(t.Elem(), visiting)
		if err != nil {
			return nil, err
		}
		schema["items"] = items
	case reflect.Map:
		items, err := schemaForType(t.Elem(), visiting)
		if err != nil {
			return nil, err
		}
		schema["additionalProperties"] = items
	case reflect.Struct:
		properties := make(map[string]any)
		var required []string
		for _, selected := range schemaJSONFields(t) {
			field := selected.field
			prop, err := schemaForType(field.Type, visiting)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", selected.name, err)
			}
			if err := parseToolTag(field.Tag.Get("tool"), prop, &required, selected.name); err != nil {
				return nil, fmt.Errorf("field %s: %w", selected.name, err)
			}
			if desc := field.Tag.Get("desc"); desc != "" {
				prop["description"] = desc
			}
			properties[selected.name] = prop
		}
		schema["properties"] = properties
		if len(required) > 0 {
			schema["required"] = required
		}
	}
	return schema, nil
}
