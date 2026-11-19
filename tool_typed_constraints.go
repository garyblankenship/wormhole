package wormhole

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// MustSchemaFromStruct is like SchemaFromStruct but panics on error.
// Use this for compile-time known types where schema generation shouldn't fail.
func MustSchemaFromStruct(v any) map[string]any {
	schema, err := SchemaFromStruct(v)
	if err != nil {
		panic(fmt.Sprintf("SchemaFromStruct failed: %v", err))
	}
	return schema
}

// goTypeToJSONType converts a Go type to a JSON Schema type string.
func goTypeToJSONType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	default:
		return "string" // Default fallback
	}
}

// parseToolTag parses the tool:"..." tag and applies constraints to the schema.
// Tag format uses semicolon as delimiter between options to allow commas in enum values.
// Examples:
//   - tool:"required"
//   - tool:"required;enum=a,b,c"
//   - tool:"min=0;max=100"
//   - tool:"enum=active,inactive,pending"
//   - tool:"enum=optional,required;required"
//
// Constraints after enum values must use semicolons; comma and pipe both
// separate enum alternatives, even when an alternative is named required.
func parseToolTag(tag string, schema map[string]any, required *[]string, fieldName string, fieldType reflect.Type) error {
	segments := strings.Split(tag, ";")
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		parts = append(parts, parseToolTagWithEnum(segment)...)
	}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		switch {
		case part == "required":
			*required = append(*required, fieldName)

		case strings.HasPrefix(part, "enum="):
			enumStr := strings.TrimPrefix(part, "enum=")
			values := strings.Split(strings.ReplaceAll(enumStr, "|", ","), ",")
			for i, v := range values {
				values[i] = strings.TrimSpace(v)
			}
			enum, err := typedEnumValues(values, fieldType)
			if err != nil {
				return err
			}
			schema["enum"] = enum

		case strings.HasPrefix(part, "min="):
			min := parseFloat(strings.TrimPrefix(part, "min="))
			if min == nil {
				return fmt.Errorf("invalid min constraint %q", part)
			}
			schema["minimum"] = *min

		case strings.HasPrefix(part, "max="):
			max := parseFloat(strings.TrimPrefix(part, "max="))
			if max == nil {
				return fmt.Errorf("invalid max constraint %q", part)
			}
			schema["maximum"] = *max

		case strings.HasPrefix(part, "minLength="):
			minLen := parseInt(strings.TrimPrefix(part, "minLength="))
			if minLen == nil {
				return fmt.Errorf("invalid minLength constraint %q", part)
			}
			schema["minLength"] = *minLen

		case strings.HasPrefix(part, "maxLength="):
			maxLen := parseInt(strings.TrimPrefix(part, "maxLength="))
			if maxLen == nil {
				return fmt.Errorf("invalid maxLength constraint %q", part)
			}
			schema["maxLength"] = *maxLen

		case strings.HasPrefix(part, "pattern="):
			schema["pattern"] = strings.TrimPrefix(part, "pattern=")

		case strings.HasPrefix(part, "default="):
			schema["default"] = strings.TrimPrefix(part, "default=")
		}
	}
	return nil
}

// parseToolTagWithEnum splits comma-separated constraints within a
// semicolon-delimited segment, preserving the remainder intact once a
// comma-containing constraint (enum=, pattern=, default=) begins.
func parseToolTagWithEnum(tag string) []string {
	var parts []string
	rest := tag
	for rest != "" {
		trimmed := strings.TrimLeft(rest, " \t")
		if strings.HasPrefix(trimmed, "enum=") ||
			strings.HasPrefix(trimmed, "pattern=") ||
			strings.HasPrefix(trimmed, "default=") {
			parts = append(parts, trimmed)
			return parts
		}
		commaIdx := strings.IndexByte(rest, ',')
		if commaIdx == -1 {
			parts = append(parts, rest)
			return parts
		}
		parts = append(parts, rest[:commaIdx])
		rest = rest[commaIdx+1:]
	}
	return parts
}

func typedEnumValues(values []string, fieldType reflect.Type) (any, error) {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	if fieldType.Kind() == reflect.String {
		return values, nil
	}
	result := make([]any, len(values))
	for i, value := range values {
		var err error
		switch fieldType.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result[i], err = strconv.ParseInt(value, 10, fieldType.Bits())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			result[i], err = strconv.ParseUint(value, 10, fieldType.Bits())
		case reflect.Float32, reflect.Float64:
			var number float64
			number, err = strconv.ParseFloat(value, fieldType.Bits())
			if err == nil && (math.IsNaN(number) || math.IsInf(number, 0)) {
				err = fmt.Errorf("value must be finite")
			}
			if fieldType.Kind() == reflect.Float32 {
				result[i] = float32(number)
			} else {
				result[i] = number
			}
		case reflect.Bool:
			result[i], err = strconv.ParseBool(value)
		default:
			return nil, fmt.Errorf("enum constraint requires a scalar field, got %s", fieldType)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid enum value %q for %s: %w", value, fieldType, err)
		}
	}
	return result, nil
}
