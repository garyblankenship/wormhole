package wormhole

import (
	"fmt"
	"sync"

	"github.com/garyblankenship/wormhole/v2/types"
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
	stored := cloneToolDefinition(definition)

	// Normalize only the registry-owned copy so callers cannot mutate registry
	// state through the definition they supplied.
	stored.Tool.Name = name
	if stored.Tool.Type == "" {
		stored.Tool.Type = "function"
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = stored
}

// Get retrieves a tool definition by name.
// Returns nil if the tool is not found.
func (r *ToolRegistry) Get(name string) *types.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return cloneToolDefinition(r.tools[name])
}

// getStored retrieves the immutable registry-owned definition for execution.
// Callers must not mutate the returned definition or its nested metadata.
func (r *ToolRegistry) getStored(name string) *types.ToolDefinition {
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
		tools = append(tools, types.CloneTool(def.Tool))
	}

	return tools
}

func cloneToolDefinition(definition *types.ToolDefinition) *types.ToolDefinition {
	if definition == nil {
		return nil
	}
	return &types.ToolDefinition{
		Tool:    types.CloneTool(definition.Tool),
		Handler: definition.Handler,
	}
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
