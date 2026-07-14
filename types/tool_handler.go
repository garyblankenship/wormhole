package types

import "context"

// ToolHandler is a function that executes a tool with the given arguments.
// It receives a context for cancellation and the arguments as a map.
// It returns the result (any type) and an error if the tool execution failed.
//
// The handler should validate arguments against the tool's schema before processing.
// If an error is returned, it will be sent back to the model as a tool error result.
//
// Example:
//
//	handler := func(ctx context.Context, args map[string]any) (any, error) {
//	    city, ok := args["city"].(string)
//	    if !ok {
//	        return nil, fmt.Errorf("city must be a string")
//	    }
//	    // Fetch weather data...
//	    return map[string]any{"temp": 72, "condition": "sunny"}, nil
//	}
type ToolHandler func(ctx context.Context, arguments map[string]any) (result any, err error)

// ToolDefinition combines a Tool schema with its execution handler.
// This type is used internally by the tool registry to store both
// the tool's metadata (name, description, schema) and its implementation.
type ToolDefinition struct {
	// Tool contains the metadata sent to the LLM (name, description, input schema)
	Tool Tool

	// Handler is the function that executes the tool when called by the model
	Handler ToolHandler
}

// NewToolDefinition creates a new ToolDefinition with the given tool and handler.
func NewToolDefinition(tool Tool, handler ToolHandler) *ToolDefinition {
	return &ToolDefinition{
		Tool:    tool,
		Handler: handler,
	}
}
