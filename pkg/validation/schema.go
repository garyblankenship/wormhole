package validation

import (
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ValidateAgainstSchema validates tool arguments against a JSON Schema
// It parses the schema map into appropriate SchemaInterface types and calls Validate
func ValidateAgainstSchema(data map[string]any, schema map[string]any) error {
	if schema == nil {
		// No schema provided, skip validation
		return nil
	}

	// Parse the schema map into a SchemaInterface
	schemaInterface, err := parseSchema(schema)
	if err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	// Validate the data against the schema
	return schemaInterface.Validate(data)
}

// parseSchema converts a map[string]any (JSON Schema) into a SchemaInterface
func parseSchema(schemaMap map[string]any) (types.SchemaInterface, error) {
	if schemaMap == nil {
		return nil, fmt.Errorf("schema map is nil")
	}

	// Check the type field to determine which schema type to create
	typeField, hasType := schemaMap["type"].(string)
	if !hasType {
		// If no type field, try to detect based on other fields
		if _, hasProperties := schemaMap["properties"]; hasProperties {
			typeField = "object"
		} else if _, hasItems := schemaMap["items"]; hasItems {
			typeField = "array"
		} else if _, hasEnum := schemaMap["enum"]; hasEnum {
			typeField = "enum"
		} else {
			// Default to object for backward compatibility
			typeField = "object"
		}
	}

	// Create schema based on type
	switch typeField {
	case "object":
		return parseObjectSchema(schemaMap)
	case "array":
		return parseArraySchema(schemaMap)
	case "string":
		return parseStringSchema(schemaMap)
	case "number":
		return parseNumberSchema(schemaMap)
	case "boolean":
		return parseBooleanSchema(schemaMap)
	case "enum":
		return parseEnumSchema(schemaMap)
	default:
		// Unknown type - attempt to parse as object as fallback
		return parseObjectSchema(schemaMap)
	}
}

// parseObjectSchema parses an object schema from a map
func parseObjectSchema(schemaMap map[string]any) (*types.ObjectSchema, error) {
	schema := &types.ObjectSchema{
		BaseSchema: types.BaseSchema{
			Type:        "object",
			Description: getString(schemaMap, "description"),
		},
	}

	// Parse required fields
	if required, ok := schemaMap["required"].([]any); ok {
		schema.Required = make([]string, len(required))
		for i, req := range required {
			if str, ok := req.(string); ok {
				schema.Required[i] = str
			}
		}
	}

	// Parse properties
	if properties, ok := schemaMap["properties"].(map[string]any); ok {
		schema.Properties = make(map[string]types.SchemaInterface)
		for propName, propSchema := range properties {
			if propMap, ok := propSchema.(map[string]any); ok {
				parsedProp, err := parseSchema(propMap)
				if err != nil {
					return nil, fmt.Errorf("failed to parse property %q: %w", propName, err)
				}
				schema.Properties[propName] = parsedProp
			} else {
				// Property schema is not a map, skip it (shouldn't happen with valid JSON schema)
				continue
			}
		}
	}

	return schema, nil
}

// parseArraySchema parses an array schema from a map
func parseArraySchema(schemaMap map[string]any) (*types.ArraySchema, error) {
	schema := &types.ArraySchema{
		BaseSchema: types.BaseSchema{
			Type:        "array",
			Description: getString(schemaMap, "description"),
		},
	}

	// Parse items
	if items, ok := schemaMap["items"]; ok {
		if itemsMap, ok := items.(map[string]any); ok {
			parsedItems, err := parseSchema(itemsMap)
			if err != nil {
				return nil, fmt.Errorf("failed to parse array items: %w", err)
			}
			schema.Items = parsedItems
		} else {
			// Items is not a map, create a simple schema based on type
			// Default to string schema for simplicity
			schema.Items = &types.StringSchema{
				BaseSchema: types.BaseSchema{
					Type: "string",
				},
			}
		}
	}

	return schema, nil
}

// parseStringSchema parses a string schema from a map
func parseStringSchema(schemaMap map[string]any) (*types.StringSchema, error) {
	return &types.StringSchema{
		BaseSchema: types.BaseSchema{
			Type:        "string",
			Description: getString(schemaMap, "description"),
		},
		MinLength: getIntPtr(schemaMap, "minLength"),
		MaxLength: getIntPtr(schemaMap, "maxLength"),
		Pattern:   getString(schemaMap, "pattern"),
	}, nil
}

// parseNumberSchema parses a number schema from a map
func parseNumberSchema(schemaMap map[string]any) (*types.NumberSchema, error) {
	return &types.NumberSchema{
		BaseSchema: types.BaseSchema{
			Type:        "number",
			Description: getString(schemaMap, "description"),
		},
		Minimum: getFloat64Ptr(schemaMap, "minimum"),
		Maximum: getFloat64Ptr(schemaMap, "maximum"),
	}, nil
}

// parseBooleanSchema parses a boolean schema from a map
func parseBooleanSchema(schemaMap map[string]any) (*types.BooleanSchema, error) {
	return &types.BooleanSchema{
		BaseSchema: types.BaseSchema{
			Type:        "boolean",
			Description: getString(schemaMap, "description"),
		},
	}, nil
}

// parseEnumSchema parses an enum schema from a map
func parseEnumSchema(schemaMap map[string]any) (*types.EnumSchema, error) {
	schema := &types.EnumSchema{
		BaseSchema: types.BaseSchema{
			Type:        "enum",
			Description: getString(schemaMap, "description"),
		},
	}

	// Parse enum values
	if enum, ok := schemaMap["enum"].([]any); ok {
		schema.Enum = enum
	}

	return schema, nil
}

// Helper functions to extract values from maps
func getString(m map[string]any, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getIntPtr(m map[string]any, key string) *int {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			intVal := int(v)
			return &intVal
		case int:
			return &v
		case int64:
			intVal := int(v)
			return &intVal
		case int32:
			intVal := int(v)
			return &intVal
		}
	}
	return nil
}

func getFloat64Ptr(m map[string]any, key string) *float64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return &v
		case float32:
			floatVal := float64(v)
			return &floatVal
		case int:
			floatVal := float64(v)
			return &floatVal
		case int64:
			floatVal := float64(v)
			return &floatVal
		}
	}
	return nil
}