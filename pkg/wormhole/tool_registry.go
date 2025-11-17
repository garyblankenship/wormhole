package wormhole

import (
	"fmt"
	"sync"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ToolRegistry manages registered tools in a thread-safe manner.
// Tools can be registered at the client level and will be available
// to all requests unless explicitly disabled.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]*types.ToolDefinition
}

// NewToolRegistry creates a new empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]*types.ToolDefinition),
	}
}

// Register adds a new tool to the registry.
// If a tool with the same name already exists, it will be replaced.
//
// Parameters:
//   - name: The unique name of the tool (must match the Tool.Name in the definition)
//   - definition: The tool definition including schema and handler
//
// Example:
//
//	registry.Register("get_weather", &types.ToolDefinition{
//	    Tool: types.Tool{
//	        Type: "function",
//	        Name: "get_weather",
//	        Description: "Get current weather",
//	        InputSchema: schema,
//	    },
//	    Handler: weatherHandler,
//	})
func (r *ToolRegistry) Register(name string, definition *types.ToolDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure the tool name matches
	if definition.Tool.Name == "" {
		definition.Tool.Name = name
	} else if definition.Tool.Name != name {
		// If name mismatch, use the provided name
		definition.Tool.Name = name
	}

	// Ensure tool type is set
	if definition.Tool.Type == "" {
		definition.Tool.Type = "function"
	}

	r.tools[name] = definition
}

// Get retrieves a tool definition by name.
// Returns nil if the tool is not found.
func (r *ToolRegistry) Get(name string) *types.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.tools[name]
}

// Has checks if a tool with the given name exists in the registry.
func (r *ToolRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.tools[name]
	return exists
}

// Unregister removes a tool from the registry.
// Returns an error if the tool doesn't exist.
func (r *ToolRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return fmt.Errorf("tool %q not found in registry", name)
	}

	delete(r.tools, name)
	return nil
}

// List returns all registered tools as a slice of Tool (without handlers).
// This is useful for passing to the LLM in requests.
func (r *ToolRegistry) List() []types.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]types.Tool, 0, len(r.tools))
	for _, def := range r.tools {
		tools = append(tools, def.Tool)
	}

	return tools
}

// ListNames returns the names of all registered tools.
func (r *ToolRegistry) ListNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}

	return names
}

// Count returns the number of registered tools.
func (r *ToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.tools)
}

// Clear removes all tools from the registry.
func (r *ToolRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools = make(map[string]*types.ToolDefinition)
}
