package wormhole

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test argument structs
type WeatherArgs struct {
	City string `json:"city" tool:"required" desc:"The city name to get weather for"`
	Unit string `json:"unit" tool:"enum=celsius,fahrenheit" desc:"Temperature unit"`
}

type WeatherResult struct {
	Temperature float64 `json:"temperature"`
	Condition   string  `json:"condition"`
}

type SearchArgs struct {
	Query    string   `json:"query" tool:"required" desc:"Search query"`
	MaxItems int      `json:"max_items" tool:"min=1,max=100" desc:"Maximum results"`
	Tags     []string `json:"tags" desc:"Filter by tags"`
}

type NumericArgs struct {
	Value    int     `json:"value" tool:"min=0,max=100" desc:"Integer value"`
	Price    float64 `json:"price" tool:"min=0.01" desc:"Price value"`
	Optional int     `json:"optional" desc:"Optional field"`
}

func TestSchemaFromStruct(t *testing.T) {
	t.Run("basic struct with json tags", func(t *testing.T) {
		type BasicArgs struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}

		schema, err := SchemaFromStruct(BasicArgs{})
		require.NoError(t, err)

		assert.Equal(t, "object", schema["type"])
		props := schema["properties"].(map[string]any)
		assert.Contains(t, props, "name")
		assert.Contains(t, props, "count")
		assert.Equal(t, "string", props["name"].(map[string]any)["type"])
		assert.Equal(t, "integer", props["count"].(map[string]any)["type"])
	})

	t.Run("required fields", func(t *testing.T) {
		schema, err := SchemaFromStruct(WeatherArgs{})
		require.NoError(t, err)

		required := schema["required"].([]string)
		assert.Contains(t, required, "city")
		assert.NotContains(t, required, "unit") // Not required
	})

	t.Run("enum constraint", func(t *testing.T) {
		schema, err := SchemaFromStruct(WeatherArgs{})
		require.NoError(t, err)

		props := schema["properties"].(map[string]any)
		unitProp := props["unit"].(map[string]any)
		assert.Contains(t, unitProp, "enum")
		enum := unitProp["enum"].([]string)
		assert.Contains(t, enum, "celsius")
		assert.Contains(t, enum, "fahrenheit")
	})

	t.Run("descriptions", func(t *testing.T) {
		schema, err := SchemaFromStruct(WeatherArgs{})
		require.NoError(t, err)

		props := schema["properties"].(map[string]any)
		cityProp := props["city"].(map[string]any)
		assert.Equal(t, "The city name to get weather for", cityProp["description"])
	})

	t.Run("numeric constraints", func(t *testing.T) {
		schema, err := SchemaFromStruct(NumericArgs{})
		require.NoError(t, err)

		props := schema["properties"].(map[string]any)

		valueProp := props["value"].(map[string]any)
		assert.Equal(t, float64(0), valueProp["minimum"])
		assert.Equal(t, float64(100), valueProp["maximum"])

		priceProp := props["price"].(map[string]any)
		assert.Equal(t, 0.01, priceProp["minimum"])
	})

	t.Run("array types", func(t *testing.T) {
		schema, err := SchemaFromStruct(SearchArgs{})
		require.NoError(t, err)

		props := schema["properties"].(map[string]any)
		tagsProp := props["tags"].(map[string]any)
		assert.Equal(t, "array", tagsProp["type"])
		assert.Equal(t, "string", tagsProp["items"].(map[string]any)["type"])
	})

	t.Run("all Go types", func(t *testing.T) {
		type AllTypes struct {
			String  string   `json:"string"`
			Int     int      `json:"int"`
			Int64   int64    `json:"int64"`
			Float   float64  `json:"float"`
			Bool    bool     `json:"bool"`
			Strings []string `json:"strings"`
		}

		schema, err := SchemaFromStruct(AllTypes{})
		require.NoError(t, err)

		props := schema["properties"].(map[string]any)
		assert.Equal(t, "string", props["string"].(map[string]any)["type"])
		assert.Equal(t, "integer", props["int"].(map[string]any)["type"])
		assert.Equal(t, "integer", props["int64"].(map[string]any)["type"])
		assert.Equal(t, "number", props["float"].(map[string]any)["type"])
		assert.Equal(t, "boolean", props["bool"].(map[string]any)["type"])
		assert.Equal(t, "array", props["strings"].(map[string]any)["type"])
	})

	t.Run("non-struct returns error", func(t *testing.T) {
		_, err := SchemaFromStruct("not a struct")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected struct")
	})

	t.Run("pointer to struct works", func(t *testing.T) {
		schema, err := SchemaFromStruct(&WeatherArgs{})
		require.NoError(t, err)
		assert.Equal(t, "object", schema["type"])
	})
}

func TestMustSchemaFromStruct(t *testing.T) {
	t.Run("valid struct doesn't panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			schema := MustSchemaFromStruct(WeatherArgs{})
			assert.NotNil(t, schema)
		})
	})

	t.Run("invalid type panics", func(t *testing.T) {
		assert.Panics(t, func() {
			MustSchemaFromStruct("not a struct")
		})
	})
}

func TestRegisterTypedTool(t *testing.T) {
	t.Run("registers tool with generated schema", func(t *testing.T) {
		client := New()

		err := RegisterTypedTool(client, "get_weather", "Get current weather",
			func(ctx context.Context, args WeatherArgs) (WeatherResult, error) {
				return WeatherResult{
					Temperature: 72.5,
					Condition:   "sunny",
				}, nil
			},
		)

		require.NoError(t, err)

		// Verify tool is registered
		assert.True(t, client.toolRegistry.Has("get_weather"))

		// Verify schema
		def := client.toolRegistry.Get("get_weather")
		require.NotNil(t, def)
		assert.Equal(t, "get_weather", def.Tool.Name)
		assert.Equal(t, "Get current weather", def.Tool.Description)
		assert.Equal(t, "function", def.Tool.Type)

		// Verify schema has correct structure
		schema := def.Tool.InputSchema
		assert.Equal(t, "object", schema["type"])
		props := schema["properties"].(map[string]any)
		assert.Contains(t, props, "city")
		assert.Contains(t, props, "unit")
	})

	t.Run("handler receives typed arguments", func(t *testing.T) {
		client := New()

		var receivedArgs WeatherArgs
		err := RegisterTypedTool(client, "weather_test", "Test",
			func(ctx context.Context, args WeatherArgs) (WeatherResult, error) {
				receivedArgs = args
				return WeatherResult{Temperature: 75}, nil
			},
		)
		require.NoError(t, err)

		// Get the handler and call it with map arguments
		def := client.toolRegistry.Get("weather_test")
		result, err := def.Handler(context.Background(), map[string]any{
			"city": "San Francisco",
			"unit": "fahrenheit",
		})

		require.NoError(t, err)
		assert.Equal(t, "San Francisco", receivedArgs.City)
		assert.Equal(t, "fahrenheit", receivedArgs.Unit)
		assert.IsType(t, WeatherResult{}, result)
		assert.Equal(t, float64(75), result.(WeatherResult).Temperature)
	})

	t.Run("handler receives array arguments", func(t *testing.T) {
		client := New()

		var receivedArgs SearchArgs
		err := RegisterTypedTool(client, "search_test", "Test",
			func(ctx context.Context, args SearchArgs) ([]string, error) {
				receivedArgs = args
				return []string{"result1", "result2"}, nil
			},
		)
		require.NoError(t, err)

		def := client.toolRegistry.Get("search_test")
		_, err = def.Handler(context.Background(), map[string]any{
			"query":     "test query",
			"max_items": float64(50), // JSON numbers are float64
			"tags":      []any{"tag1", "tag2"},
		})

		require.NoError(t, err)
		assert.Equal(t, "test query", receivedArgs.Query)
		assert.Equal(t, 50, receivedArgs.MaxItems)
		assert.Equal(t, []string{"tag1", "tag2"}, receivedArgs.Tags)
	})

	t.Run("handler error is propagated", func(t *testing.T) {
		client := New()

		expectedErr := assert.AnError
		err := RegisterTypedTool(client, "error_test", "Test",
			func(ctx context.Context, args WeatherArgs) (WeatherResult, error) {
				return WeatherResult{}, expectedErr
			},
		)
		require.NoError(t, err)

		def := client.toolRegistry.Get("error_test")
		_, err = def.Handler(context.Background(), map[string]any{
			"city": "Test",
		})

		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestToolTagParsing(t *testing.T) {
	t.Run("multiple constraints", func(t *testing.T) {
		type MultiConstraint struct {
			Value int `json:"value" tool:"required,min=0,max=100" desc:"A constrained value"`
		}

		schema, err := SchemaFromStruct(MultiConstraint{})
		require.NoError(t, err)

		// Check required
		required := schema["required"].([]string)
		assert.Contains(t, required, "value")

		// Check numeric constraints
		props := schema["properties"].(map[string]any)
		valueProp := props["value"].(map[string]any)
		assert.Equal(t, float64(0), valueProp["minimum"])
		assert.Equal(t, float64(100), valueProp["maximum"])
		assert.Equal(t, "A constrained value", valueProp["description"])
	})

	t.Run("enum with pipe separator", func(t *testing.T) {
		type EnumTest struct {
			Status string `json:"status" tool:"enum=active|inactive|pending"`
		}

		schema, err := SchemaFromStruct(EnumTest{})
		require.NoError(t, err)

		props := schema["properties"].(map[string]any)
		statusProp := props["status"].(map[string]any)
		enum := statusProp["enum"].([]string)
		assert.ElementsMatch(t, []string{"active", "inactive", "pending"}, enum)
	})

	t.Run("string constraints", func(t *testing.T) {
		type StringConstraints struct {
			Code    string `json:"code" tool:"minLength=3,maxLength=10"`
			Pattern string `json:"pattern" tool:"pattern=^[A-Z]+$"`
		}

		schema, err := SchemaFromStruct(StringConstraints{})
		require.NoError(t, err)

		props := schema["properties"].(map[string]any)

		codeProp := props["code"].(map[string]any)
		assert.Equal(t, 3, codeProp["minLength"])
		assert.Equal(t, 10, codeProp["maxLength"])

		patternProp := props["pattern"].(map[string]any)
		assert.Equal(t, "^[A-Z]+$", patternProp["pattern"])
	})
}
