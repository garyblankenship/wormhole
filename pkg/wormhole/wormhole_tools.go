package wormhole

import (
	"encoding/json"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// RegisterTool registers a new tool that can be called by LLMs.
func (p *Wormhole) RegisterTool(name string, description string, schema types.Schema, handler types.ToolHandler) {
	var schemaMap map[string]any

	if m, ok := schema.(map[string]any); ok {
		schemaMap = m
	} else {
		schemaJSON, err := json.Marshal(schema)
		if err != nil {
			schemaMap = make(map[string]any)
		} else if err := json.Unmarshal(schemaJSON, &schemaMap); err != nil {
			schemaMap = make(map[string]any)
		}
	}

	tool := types.Tool{
		Type:        "function",
		Name:        name,
		Description: description,
		InputSchema: schemaMap,
		Function: &types.ToolFunction{
			Name:        name,
			Description: description,
			Parameters:  schemaMap,
		},
	}

	definition := types.NewToolDefinition(tool, handler)
	p.toolRegistry.Register(name, definition)
}

// UnregisterTool removes a tool from the registry.
func (p *Wormhole) UnregisterTool(name string) error {
	return p.toolRegistry.Unregister(name)
}

// ListTools returns all registered tools.
func (p *Wormhole) ListTools() []types.Tool {
	return p.toolRegistry.List()
}

// HasTool checks if a tool with the given name is registered.
func (p *Wormhole) HasTool(name string) bool {
	return p.toolRegistry.Has(name)
}

// ToolCount returns the number of registered tools.
func (p *Wormhole) ToolCount() int {
	return p.toolRegistry.Count()
}

// ClearTools removes all registered tools.
func (p *Wormhole) ClearTools() {
	p.toolRegistry.Clear()
}
