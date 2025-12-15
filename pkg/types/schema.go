package types

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// Schema represents a structured output schema interface or raw JSON bytes
type Schema any

// SchemaInterface represents the original schema interface
type SchemaInterface interface {
	GetType() string
	GetDescription() string
	Validate(data any) error
}

// BaseSchema provides common schema functionality
type BaseSchema struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

func (s *BaseSchema) GetType() string {
	return s.Type
}

func (s *BaseSchema) GetDescription() string {
	return s.Description
}

// ObjectSchema represents an object schema
type ObjectSchema struct {
	BaseSchema
	Properties map[string]SchemaInterface `json:"properties"`
	Required   []string                   `json:"required,omitempty"`
}

func (s *ObjectSchema) Validate(data any) error {
	if data == nil {
		return NewWormholeError(ErrorCodeValidation, "data cannot be nil", false)
	}

	value := reflect.ValueOf(data)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	if value.Kind() != reflect.Map && value.Kind() != reflect.Struct {
		return NewWormholeError(ErrorCodeValidation, "data must be an object", false)
	}

	// Convert to map for unified processing
	dataMap := toDataMap(value)

	// Check required fields
	if err := s.validateRequired(dataMap); err != nil {
		return err
	}

	// Validate properties
	return s.validateProperties(dataMap)
}

// toDataMap converts a reflect.Value (map or struct) to map[string]any
func toDataMap(value reflect.Value) map[string]any {
	dataMap := make(map[string]any)

	if value.Kind() == reflect.Map {
		for _, key := range value.MapKeys() {
			dataMap[fmt.Sprintf("%v", key.Interface())] = value.MapIndex(key).Interface()
		}
		return dataMap
	}

	// Struct to map conversion
	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		field := valueType.Field(i)
		fieldName := getFieldName(field)
		dataMap[fieldName] = value.Field(i).Interface()
	}
	return dataMap
}

// getFieldName extracts the field name from struct field, preferring json tag
func getFieldName(field reflect.StructField) string {
	jsonTag := field.Tag.Get("json")
	if jsonTag == "" || jsonTag == "-" {
		return field.Name
	}
	parts := strings.Split(jsonTag, ",")
	if parts[0] != "" {
		return parts[0]
	}
	return field.Name
}

// validateRequired checks that all required fields are present
func (s *ObjectSchema) validateRequired(dataMap map[string]any) error {
	for _, req := range s.Required {
		if _, exists := dataMap[req]; !exists {
			return NewWormholeError(ErrorCodeValidation, fmt.Sprintf("required field '%s' is missing", req), false)
		}
	}
	return nil
}

// validateProperties validates each property against its schema
func (s *ObjectSchema) validateProperties(dataMap map[string]any) error {
	for propName, propSchema := range s.Properties {
		if propValue, exists := dataMap[propName]; exists {
			if err := propSchema.Validate(propValue); err != nil {
				return NewWormholeError(ErrorCodeValidation, fmt.Sprintf("property '%s': %v", propName, err), false)
			}
		}
	}
	return nil
}

// ArraySchema represents an array schema
type ArraySchema struct {
	BaseSchema
	Items SchemaInterface `json:"items"`
}

func (s *ArraySchema) Validate(data any) error {
	if data == nil {
		return NewWormholeError(ErrorCodeValidation, "data cannot be nil", false)
	}

	value := reflect.ValueOf(data)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	if value.Kind() != reflect.Slice && value.Kind() != reflect.Array {
		return NewWormholeError(ErrorCodeValidation, "data must be an array", false)
	}

	// Validate each item
	for i := 0; i < value.Len(); i++ {
		item := value.Index(i).Interface()
		if err := s.Items.Validate(item); err != nil {
			return NewWormholeError(ErrorCodeValidation, fmt.Sprintf("item at index %d: %v", i, err), false)
		}
	}

	return nil
}

// StringSchema represents a string schema
type StringSchema struct {
	BaseSchema
	MinLength *int   `json:"minLength,omitempty"`
	MaxLength *int   `json:"maxLength,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
}

func (s *StringSchema) Validate(data any) error {
	if data == nil {
		return NewWormholeError(ErrorCodeValidation, "data cannot be nil", false)
	}

	str, ok := data.(string)
	if !ok {
		return NewWormholeError(ErrorCodeValidation, "data must be a string", false)
	}

	// Check length constraints
	if s.MinLength != nil && len(str) < *s.MinLength {
		return NewWormholeError(ErrorCodeValidation, fmt.Sprintf("string length %d is less than minimum %d", len(str), *s.MinLength), false)
	}

	if s.MaxLength != nil && len(str) > *s.MaxLength {
		return NewWormholeError(ErrorCodeValidation, fmt.Sprintf("string length %d exceeds maximum %d", len(str), *s.MaxLength), false)
	}

	// Check pattern
	if s.Pattern != "" {
		matched, err := regexp.MatchString(s.Pattern, str)
		if err != nil {
			return NewWormholeError(ErrorCodeValidation, fmt.Sprintf("invalid pattern: %v", err), false)
		}
		if !matched {
			return NewWormholeError(ErrorCodeValidation, fmt.Sprintf("string does not match pattern '%s'", s.Pattern), false)
		}
	}

	return nil
}

// NumberSchema represents a number schema
type NumberSchema struct {
	BaseSchema
	Minimum *float64 `json:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
}

func (s *NumberSchema) Validate(data any) error {
	if data == nil {
		return NewWormholeError(ErrorCodeValidation, "data cannot be nil", false)
	}

	var num float64
	switch v := data.(type) {
	case float64:
		num = v
	case float32:
		num = float64(v)
	case int:
		num = float64(v)
	case int32:
		num = float64(v)
	case int64:
		num = float64(v)
	default:
		return NewWormholeError(ErrorCodeValidation, "data must be a number", false)
	}

	// Check range constraints
	if s.Minimum != nil && num < *s.Minimum {
		return NewWormholeError(ErrorCodeValidation, fmt.Sprintf("number %.2f is less than minimum %.2f", num, *s.Minimum), false)
	}

	if s.Maximum != nil && num > *s.Maximum {
		return NewWormholeError(ErrorCodeValidation, fmt.Sprintf("number %.2f exceeds maximum %.2f", num, *s.Maximum), false)
	}

	return nil
}

// BooleanSchema represents a boolean schema
type BooleanSchema struct {
	BaseSchema
}

func (s *BooleanSchema) Validate(data any) error {
	if data == nil {
		return NewWormholeError(ErrorCodeValidation, "data cannot be nil", false)
	}

	if _, ok := data.(bool); !ok {
		return NewWormholeError(ErrorCodeValidation, "data must be a boolean", false)
	}

	return nil
}

// EnumSchema represents an enum schema
type EnumSchema struct {
	BaseSchema
	Enum []any `json:"enum"`
}

func (s *EnumSchema) Validate(data any) error {
	if data == nil {
		return NewWormholeError(ErrorCodeValidation, "data cannot be nil", false)
	}

	// Check if data matches any enum value
	for _, enumValue := range s.Enum {
		if reflect.DeepEqual(data, enumValue) {
			return nil
		}
	}

	return NewWormholeError(ErrorCodeValidation, fmt.Sprintf("value does not match any enum option: %v", s.Enum), false)
}
