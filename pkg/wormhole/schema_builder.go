package wormhole

import (
	"github.com/garyblankenship/wormhole/pkg/types"
)

// ==================== Object Schema Builder ====================

// ObjectSchemaBuilder provides a fluent interface for constructing ObjectSchema
type ObjectSchemaBuilder struct {
	schema types.ObjectSchema
}

// NewObjectSchema creates a new ObjectSchemaBuilder
func NewObjectSchema() *ObjectSchemaBuilder {
	return &ObjectSchemaBuilder{
		schema: types.ObjectSchema{
			BaseSchema: types.BaseSchema{
				Type: "object",
			},
			Properties: make(map[string]types.SchemaInterface),
			Required:   make([]string, 0),
		},
	}
}

// Description sets the description for the object schema
func (b *ObjectSchemaBuilder) Description(description string) *ObjectSchemaBuilder {
	b.schema.Description = description
	return b
}

// Property adds a property to the object schema
func (b *ObjectSchemaBuilder) Property(name string, schema types.SchemaInterface) *ObjectSchemaBuilder {
	b.schema.Properties[name] = schema
	return b
}

// Required marks a property as required
func (b *ObjectSchemaBuilder) Required(name string) *ObjectSchemaBuilder {
	// Check if already in required list
	for _, req := range b.schema.Required {
		if req == name {
			return b
		}
	}
	b.schema.Required = append(b.schema.Required, name)
	return b
}

// Build returns the constructed ObjectSchema
func (b *ObjectSchemaBuilder) Build() types.ObjectSchema {
	return b.schema
}

// ==================== String Schema Builder ====================

// StringSchemaBuilder provides a fluent interface for constructing StringSchema
type StringSchemaBuilder struct {
	schema types.StringSchema
}

// NewStringSchema creates a new StringSchemaBuilder
func NewStringSchema() *StringSchemaBuilder {
	return &StringSchemaBuilder{
		schema: types.StringSchema{
			BaseSchema: types.BaseSchema{
				Type: "string",
			},
		},
	}
}

// Description sets the description for the string schema
func (b *StringSchemaBuilder) Description(description string) *StringSchemaBuilder {
	b.schema.Description = description
	return b
}

// MinLength sets the minimum length constraint
func (b *StringSchemaBuilder) MinLength(min int) *StringSchemaBuilder {
	b.schema.MinLength = &min
	return b
}

// MaxLength sets the maximum length constraint
func (b *StringSchemaBuilder) MaxLength(max int) *StringSchemaBuilder {
	b.schema.MaxLength = &max
	return b
}

// Pattern sets the regex pattern constraint
func (b *StringSchemaBuilder) Pattern(pattern string) *StringSchemaBuilder {
	b.schema.Pattern = pattern
	return b
}

// Build returns the constructed StringSchema
func (b *StringSchemaBuilder) Build() types.StringSchema {
	return b.schema
}

// ==================== Number Schema Builder ====================

// NumberSchemaBuilder provides a fluent interface for constructing NumberSchema
type NumberSchemaBuilder struct {
	schema types.NumberSchema
}

// NewNumberSchema creates a new NumberSchemaBuilder
func NewNumberSchema() *NumberSchemaBuilder {
	return &NumberSchemaBuilder{
		schema: types.NumberSchema{
			BaseSchema: types.BaseSchema{
				Type: "number",
			},
		},
	}
}

// Description sets the description for the number schema
func (b *NumberSchemaBuilder) Description(description string) *NumberSchemaBuilder {
	b.schema.Description = description
	return b
}

// Minimum sets the minimum value constraint
func (b *NumberSchemaBuilder) Minimum(min float64) *NumberSchemaBuilder {
	b.schema.Minimum = &min
	return b
}

// Maximum sets the maximum value constraint
func (b *NumberSchemaBuilder) Maximum(max float64) *NumberSchemaBuilder {
	b.schema.Maximum = &max
	return b
}

// Build returns the constructed NumberSchema
func (b *NumberSchemaBuilder) Build() types.NumberSchema {
	return b.schema
}

// ==================== Boolean Schema Builder ====================

// BooleanSchemaBuilder provides a fluent interface for constructing BooleanSchema
type BooleanSchemaBuilder struct {
	schema types.BooleanSchema
}

// NewBooleanSchema creates a new BooleanSchemaBuilder
func NewBooleanSchema() *BooleanSchemaBuilder {
	return &BooleanSchemaBuilder{
		schema: types.BooleanSchema{
			BaseSchema: types.BaseSchema{
				Type: "boolean",
			},
		},
	}
}

// Description sets the description for the boolean schema
func (b *BooleanSchemaBuilder) Description(description string) *BooleanSchemaBuilder {
	b.schema.Description = description
	return b
}

// Build returns the constructed BooleanSchema
func (b *BooleanSchemaBuilder) Build() types.BooleanSchema {
	return b.schema
}

// ==================== Array Schema Builder ====================

// ArraySchemaBuilder provides a fluent interface for constructing ArraySchema
type ArraySchemaBuilder struct {
	schema types.ArraySchema
}

// NewArraySchema creates a new ArraySchemaBuilder
func NewArraySchema(items types.SchemaInterface) *ArraySchemaBuilder {
	return &ArraySchemaBuilder{
		schema: types.ArraySchema{
			BaseSchema: types.BaseSchema{
				Type: "array",
			},
			Items: items,
		},
	}
}

// Description sets the description for the array schema
func (b *ArraySchemaBuilder) Description(description string) *ArraySchemaBuilder {
	b.schema.Description = description
	return b
}

// Build returns the constructed ArraySchema
func (b *ArraySchemaBuilder) Build() types.ArraySchema {
	return b.schema
}

// ==================== Enum Schema Builder ====================

// EnumSchemaBuilder provides a fluent interface for constructing EnumSchema
type EnumSchemaBuilder struct {
	schema types.EnumSchema
}

// NewEnumSchema creates a new EnumSchemaBuilder with the provided enum values
func NewEnumSchema(values ...any) *EnumSchemaBuilder {
	return &EnumSchemaBuilder{
		schema: types.EnumSchema{
			BaseSchema: types.BaseSchema{
				Type: "string", // Most enums are string-based
			},
			Enum: values,
		},
	}
}

// Description sets the description for the enum schema
func (b *EnumSchemaBuilder) Description(description string) *EnumSchemaBuilder {
	b.schema.Description = description
	return b
}

// Type sets the type for the enum (default is "string")
func (b *EnumSchemaBuilder) Type(t string) *EnumSchemaBuilder {
	b.schema.Type = t
	return b
}

// Build returns the constructed EnumSchema
func (b *EnumSchemaBuilder) Build() types.EnumSchema {
	return b.schema
}

// ==================== Convenience Functions ====================

// String creates a simple string schema (shorthand for NewStringSchema().Build())
func String() types.StringSchema {
	return NewStringSchema().Build()
}

// StringWithDesc creates a string schema with description
func StringWithDesc(description string) types.StringSchema {
	return NewStringSchema().Description(description).Build()
}

// Number creates a simple number schema
func Number() types.NumberSchema {
	return NewNumberSchema().Build()
}

// NumberWithDesc creates a number schema with description
func NumberWithDesc(description string) types.NumberSchema {
	return NewNumberSchema().Description(description).Build()
}

// Boolean creates a simple boolean schema
func Boolean() types.BooleanSchema {
	return NewBooleanSchema().Build()
}

// BooleanWithDesc creates a boolean schema with description
func BooleanWithDesc(description string) types.BooleanSchema {
	return NewBooleanSchema().Description(description).Build()
}

// Enum creates an enum schema with the provided values
func Enum(values ...any) types.EnumSchema {
	return NewEnumSchema(values...).Build()
}

// StringEnum creates a string enum schema (convenience for string-based enums)
func StringEnum(values ...string) types.EnumSchema {
	anyValues := make([]any, len(values))
	for i, v := range values {
		anyValues[i] = v
	}
	return NewEnumSchema(anyValues...).Build()
}
