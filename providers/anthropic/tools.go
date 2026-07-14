package anthropic

import (
	"encoding/json"

	"github.com/garyblankenship/wormhole/v2/types"
)

// transformTools converts internal tools to Anthropic format
func (p *Provider) transformTools(tools []types.Tool) []map[string]any {
	result := make([]map[string]any, len(tools))

	for i, tool := range tools {
		name := tool.Name
		description := tool.Description
		var schema any = tool.InputSchema
		if tool.Function != nil {
			name = tool.Function.Name
			description = tool.Function.Description
			schema = tool.Function.Parameters
		}
		parameters, _ := json.Marshal(schema)
		result[i] = map[string]any{
			"name":         name,
			"description":  description,
			"input_schema": json.RawMessage(parameters),
		}
	}

	return result
}

// transformToolChoice converts the internal tool choice to Anthropic's wire format.
func (p *Provider) transformToolChoice(tc *types.ToolChoice) map[string]any {
	switch tc.Type {
	case types.ToolChoiceTypeSpecific:
		if tc.ToolName != "" {
			return map[string]any{"type": "tool", "name": tc.ToolName}
		}
		return map[string]any{"type": "auto"}
	case types.ToolChoiceTypeAny:
		return map[string]any{"type": "any"}
	case types.ToolChoiceTypeNone:
		return map[string]any{"type": "none"}
	default:
		return map[string]any{"type": "auto"}
	}
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
