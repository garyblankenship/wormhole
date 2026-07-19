package wormhole

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/v2/types"
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
