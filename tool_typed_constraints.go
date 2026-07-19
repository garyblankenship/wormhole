package wormhole

import (
	"fmt"
	"reflect"
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
func parseToolTag(tag string, schema map[string]any, required *[]string, fieldName string) {
	// Split by semicolon first (preferred delimiter)
	// Fall back to comma only if no semicolon found AND no enum= present
	var parts []string
	if strings.Contains(tag, ";") {
		parts = strings.Split(tag, ";")
	} else if strings.Contains(tag, "enum=") {
		// Special handling: if there's an enum, parse carefully
		parts = parseToolTagWithEnum(tag)
	} else {
		parts = strings.Split(tag, ",")
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
			// Enum values can use either comma or pipe as separator
			enumStr := strings.TrimPrefix(part, "enum=")
			var values []string
			if strings.Contains(enumStr, "|") {
				values = strings.Split(enumStr, "|")
			} else {
				values = strings.Split(enumStr, ",")
			}
			// Clean up values
			for i, v := range values {
				values[i] = strings.TrimSpace(v)
			}
			schema["enum"] = values

		case strings.HasPrefix(part, "min="):
			if min := parseFloat(strings.TrimPrefix(part, "min=")); min != nil {
				schema["minimum"] = *min
			}

		case strings.HasPrefix(part, "max="):
			if max := parseFloat(strings.TrimPrefix(part, "max=")); max != nil {
				schema["maximum"] = *max
			}

		case strings.HasPrefix(part, "minLength="):
			if minLen := parseInt(strings.TrimPrefix(part, "minLength=")); minLen != nil {
				schema["minLength"] = *minLen
			}

		case strings.HasPrefix(part, "maxLength="):
			if maxLen := parseInt(strings.TrimPrefix(part, "maxLength=")); maxLen != nil {
				schema["maxLength"] = *maxLen
			}

		case strings.HasPrefix(part, "pattern="):
			schema["pattern"] = strings.TrimPrefix(part, "pattern=")

		case strings.HasPrefix(part, "default="):
			schema["default"] = strings.TrimPrefix(part, "default=")
		}
	}
}

// parseToolTagWithEnum handles the special case of parsing tool tags that contain enum=.
// It extracts the enum part separately to preserve comma-separated enum values.
func parseToolTagWithEnum(tag string) []string {
	var parts []string
	enumIdx := strings.Index(tag, "enum=")

	if enumIdx == -1 {
		return strings.Split(tag, ",")
	}

	// Extract parts before enum=
	if enumIdx > 0 {
		before := strings.TrimRight(tag[:enumIdx], ",")
		if before != "" {
			parts = append(parts, strings.Split(before, ",")...)
		}
	}

	// Find the enum value - it continues until end or until next constraint keyword
	enumStart := enumIdx
	enumEnd := len(tag)

	// Look for the next constraint marker after enum values
	// Common patterns: ";min=", ";max=", ";pattern=", etc.
	remainder := tag[enumIdx:]
	constraintMarkers := []string{";min=", ";max=", ";minLength=", ";maxLength=", ";pattern=", ";default=", ";required"}

	for _, marker := range constraintMarkers {
		if idx := strings.Index(remainder, marker); idx > 0 {
			if idx < enumEnd-enumIdx {
				enumEnd = enumIdx + idx
			}
		}
	}

	// Also check for comma followed by a known constraint keyword (legacy format)
	knownConstraints := []string{",min=", ",max=", ",minLength=", ",maxLength=", ",pattern=", ",default=", ",required"}
	for _, marker := range knownConstraints {
		if idx := strings.Index(remainder, marker); idx > 0 {
			if idx < enumEnd-enumIdx {
				enumEnd = enumIdx + idx
			}
		}
	}

	// Add the enum part
	parts = append(parts, tag[enumStart:enumEnd])

	// Extract parts after enum
	if enumEnd < len(tag) {
		after := strings.TrimLeft(tag[enumEnd:], ",;")
		if after != "" {
			parts = append(parts, strings.Split(after, ",")...)
		}
	}

	return parts
}
