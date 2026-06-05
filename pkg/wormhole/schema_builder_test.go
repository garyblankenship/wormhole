package wormhole

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaBuilders(t *testing.T) {
	t.Parallel()
	nameSchema := NewStringSchema().
		Description("person name").
		MinLength(2).
		MaxLength(20).
		Pattern(`^[A-Z]`).
		Build()
	ageSchema := NewNumberSchema().
		Description("person age").
		Minimum(0).
		Maximum(130).
		Build()
	activeSchema := NewBooleanSchema().
		Description("active flag").
		Build()
	roleSchema := NewEnumSchema("admin", "user").
		Description("role").
		Type("string").
		Build()

	objectSchema := NewObjectSchema().
		Description("person").
		Property("name", &nameSchema).
		Property("age", &ageSchema).
		Property("active", &activeSchema).
		Property("role", &roleSchema).
		Required("name").
		Required("name").
		Required("role").
		Build()

	assert.Equal(t, "object", objectSchema.Type)
	assert.Equal(t, "person", objectSchema.Description)
	assert.Len(t, objectSchema.Properties, 4)
	assert.Equal(t, []string{"name", "role"}, objectSchema.Required)
	require.NoError(t, objectSchema.Validate(map[string]any{
		"name":   "Alice",
		"age":    42,
		"active": true,
		"role":   "admin",
	}))
	require.Error(t, objectSchema.Validate(map[string]any{"name": "alice", "role": "admin"}))
	require.Error(t, objectSchema.Validate(map[string]any{"name": "Alice"}))
}

func TestArrayAndConvenienceSchemaBuilders(t *testing.T) {
	t.Parallel()
	itemSchema := StringWithDesc("tag")
	arraySchema := NewArraySchema(&itemSchema).
		Description("tags").
		Build()

	assert.Equal(t, "array", arraySchema.Type)
	assert.Equal(t, "tags", arraySchema.Description)
	require.NoError(t, arraySchema.Validate([]string{"alpha", "beta"}))
	require.Error(t, arraySchema.Validate([]int{1, 2}))

	stringSchema := String()
	assert.Equal(t, "string", stringSchema.GetType())

	numberSchema := NumberWithDesc("score")
	assert.Equal(t, "number", numberSchema.GetType())
	assert.Equal(t, "score", numberSchema.GetDescription())

	booleanSchema := BooleanWithDesc("enabled")
	assert.Equal(t, "boolean", booleanSchema.GetType())
	assert.Equal(t, "enabled", booleanSchema.GetDescription())

	enumSchema := Enum("small", "large")
	require.NoError(t, enumSchema.Validate("small"))
	require.Error(t, enumSchema.Validate("medium"))

	stringEnum := StringEnum("red", "blue")
	assert.Equal(t, []any{"red", "blue"}, stringEnum.Enum)

	var _ types.SchemaInterface = &stringSchema
	var _ types.SchemaInterface = &numberSchema
	var _ types.SchemaInterface = &booleanSchema
	var _ types.SchemaInterface = &enumSchema
}

func TestSimpleConvenienceSchemas(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "name", StringWithDesc("name").Description)
	assert.Equal(t, "number", Number().Type)
	assert.Equal(t, "boolean", Boolean().Type)
}
