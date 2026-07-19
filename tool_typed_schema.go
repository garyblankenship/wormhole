package wormhole

import (
	"fmt"
	"reflect"
	"strings"
)

// SchemaFromStruct generates a JSON Schema from a struct type using reflection.
// This is useful when you need the schema separately from tool registration.
//
// Example:
//
//	type SearchArgs struct {
//	    Query    string   `json:"query" tool:"required" desc:"Search query"`
//	    MaxItems int      `json:"max_items" tool:"min=1,max=100" desc:"Maximum results"`
//	    Tags     []string `json:"tags" desc:"Filter by tags"`
//	}
//
//	schema := wormhole.SchemaFromStruct(SearchArgs{})
func SchemaFromStruct(v any) (map[string]any, error) {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %s", t.Kind())
	}

	properties := make(map[string]any)
	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON field name
		jsonTag := field.Tag.Get("json")
		fieldName := field.Name
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" && parts[0] != "-" {
				fieldName = parts[0]
			} else if parts[0] == "-" {
				continue // Skip this field
			}
		}

		// Build property schema
		propSchema := make(map[string]any)

		// Set type based on Go type
		propSchema["type"] = goTypeToJSONType(field.Type)

		// Handle array types
		if field.Type.Kind() == reflect.Slice {
			itemType := goTypeToJSONType(field.Type.Elem())
			propSchema["items"] = map[string]any{"type": itemType}
		}

		// Parse tool tag for constraints
		toolTag := field.Tag.Get("tool")
		if toolTag != "" {
			parseToolTag(toolTag, propSchema, &required, fieldName)
		}

		// Add description from desc tag
		if desc := field.Tag.Get("desc"); desc != "" {
			propSchema["description"] = desc
		}

		properties[fieldName] = propSchema
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema, nil
}
