package wormhole

import (
	"context"
	"encoding/json"
	"fmt"
)

// mergeTools creates a merged registry with global + agent-scoped tools.
// Agent tools override global tools with the same name.
func (b *AgentBuilder) mergeTools() *ToolRegistry {
	merged := NewToolRegistry()

	// Copy global tools first
	globalTools := b.wormhole.toolRegistry
	for _, name := range globalTools.ListNames() {
		def := globalTools.Get(name)
		if def != nil {
			merged.Register(name, def)
		}
	}

	// Override with agent-scoped tools
	for _, name := range b.tools.ListNames() {
		def := b.tools.Get(name)
		if def != nil {
			merged.Register(name, def)
		}
	}

	return merged
}

// AgentAddTool registers a type-safe tool on the AgentBuilder.
// This is the agent-scoped equivalent of RegisterTypedTool.
//
// Example:
//
//	type SearchArgs struct {
//	    Query string `json:"query" tool:"required" desc:"Search query"`
//	}
//
//	builder := client.Agent().Model("gpt-5.2")
//	wormhole.AgentAddTool(builder, "search", "Search the web",
//	    func(ctx context.Context, args SearchArgs) (SearchResult, error) {
//	        return search(args.Query), nil
//	    },
//	)
//	result, _ := builder.Run(ctx, "Find Go 1.23 release notes")
func AgentAddTool[Args any, Result any](
	builder *AgentBuilder,
	name string,
	description string,
	handler func(ctx context.Context, args Args) (Result, error),
) error {
	var args Args
	schema, err := SchemaFromStruct(args)
	if err != nil {
		return fmt.Errorf("failed to generate schema for tool %q: %w", name, err)
	}

	wrappedHandler := func(ctx context.Context, arguments map[string]any) (any, error) {
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

	builder.AddTool(name, description, schema, wrappedHandler)
	return nil
}
