package wormhole

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// RegisterTypedTool registers a type-safe tool with automatic schema generation.
// This dramatically reduces boilerplate by inferring the JSON schema from the handler's
// argument struct using reflection and struct tags.
//
// Struct tags supported:
//   - `json:"field_name"` - JSON property name (standard encoding/json tag)
//   - `tool:"required"` - Mark field as required
//   - `tool:"enum=a,b,c"` - Enum constraint (comma-separated values)
//   - `tool:"min=0"` - Minimum numeric value
//   - `tool:"max=100"` - Maximum numeric value
//   - `desc:"description"` - Field description for the LLM
//
// Example:
//
//	type WeatherArgs struct {
//	    City string `json:"city" tool:"required" desc:"The city name"`
//	    Unit string `json:"unit" tool:"enum=celsius,fahrenheit" desc:"Temperature unit"`
//	}
//
//	wormhole.RegisterTypedTool(client, "get_weather", "Get current weather",
//	    func(ctx context.Context, args WeatherArgs) (WeatherResult, error) {
//	        return getWeather(args.City, args.Unit), nil
//	    },
//	)
//
// The handler function signature must be:
//
//	func(ctx context.Context, args T) (result R, err error)
//
// where T is any struct type that will be used to generate the JSON schema.
func RegisterTypedTool[Args any, Result any](
	client *Wormhole,
	name string,
	description string,
	handler func(ctx context.Context, args Args) (Result, error),
) error {
	// Generate schema from the Args type
	var args Args
	schema, err := SchemaFromStruct(args)
	if err != nil {
		return fmt.Errorf("failed to generate schema for tool %q: %w", name, err)
	}

	// Create a wrapper handler that unmarshals map[string]any to the typed struct
	wrappedHandler := func(ctx context.Context, arguments map[string]any) (any, error) {
		// Convert map to JSON and back to typed struct
		// This ensures proper type conversion and validation
		jsonBytes, err := json.Marshal(arguments)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal arguments: %w", err)
		}

		var typedArgs Args
		if err := json.Unmarshal(jsonBytes, &typedArgs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal arguments to %T: %w", typedArgs, err)
		}

		return handler(ctx, typedArgs)
	}

	// Register with the existing registry
	client.toolRegistry.Register(name, &types.ToolDefinition{
		Tool: types.Tool{
			Type:        "function",
			Name:        name,
			Description: description,
			InputSchema: schema,
		},
		Handler: wrappedHandler,
	})

	return nil
}

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
	if t.Kind() == reflect.Ptr {
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

// parseFloat parses a string to float64, returning nil if parsing fails.
func parseFloat(s string) *float64 {
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
		return &f
	}
	return nil
}

// parseInt parses a string to int, returning nil if parsing fails.
func parseInt(s string) *int {
	var i int
	if _, err := fmt.Sscanf(s, "%d", &i); err == nil {
		return &i
	}
	return nil
}
