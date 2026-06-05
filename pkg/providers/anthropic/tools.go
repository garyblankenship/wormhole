package anthropic

import (
	"encoding/json"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// transformTools converts internal tools to Anthropic format
func (p *Provider) transformTools(tools []types.Tool) []map[string]any {
	result := make([]map[string]any, len(tools))

	for i, tool := range tools {
		parameters, _ := json.Marshal(tool.Function.Parameters)
		result[i] = map[string]any{
			"name":         tool.Function.Name,
			"description":  tool.Function.Description,
			"input_schema": json.RawMessage(parameters),
		}
	}

	return result
}

// schemaToTool converts a JSON schema to a Tool suitable for structured output
func (p *Provider) schemaToTool(schema json.RawMessage, name string) (*types.Tool, error) {
	if name == "" {
		name = "structured_output"
	}

	// Convert json.RawMessage to map[string]any
	var params map[string]any
	if err := json.Unmarshal(schema, &params); err != nil {
		return nil, err
	}

	return &types.Tool{
		Type: "function",
		Function: &types.ToolFunction{
			Name:        name,
			Description: "Extract structured data",
			Parameters:  params,
		},
	}, nil
}
