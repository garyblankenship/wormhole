package gemini

import (
	"github.com/garyblankenship/wormhole/v2/types"
)

// transformTools converts tools to Gemini format
func (g *Gemini) transformTools(tools []types.Tool) []map[string]any {
	// Use shared RequestBuilder for common tool transformation
	standardTools := g.requestBuilder.TransformTools(tools)

	// Adapt to Gemini-specific format: extract function declarations
	functions := make([]map[string]any, 0, len(standardTools))
	for _, stdTool := range standardTools {
		if toolFunc, ok := stdTool["function"].(map[string]any); ok {
			// Extract name, description, parameters from standard tool format
			function := map[string]any{
				"name":        toolFunc["name"],
				"description": toolFunc["description"],
			}

			// Handle parameters - ensure type: "object" if missing
			if params, ok := toolFunc["parameters"].(map[string]any); ok {
				params = g.transformToolSchema(params)
				function["parameters"] = params
			}

			functions = append(functions, function)
		}
	}

	// Wrap functions in Gemini format
	var geminiTools []map[string]any
	if len(functions) > 0 {
		geminiTools = append(geminiTools, map[string]any{
			"functionDeclarations": functions,
		})
	}

	return geminiTools
}

// transformToolSchema converts tool schema to Gemini format
func (g *Gemini) transformToolSchema(schema map[string]any) map[string]any {
	// Gemini expects JSON Schema format
	if _, ok := schema["type"]; !ok {
		schema = types.CloneMap(schema)
		schema["type"] = "object"
	}
	return normalizeSchemaMap(schema)
}

// transformToolChoice converts tool choice to Gemini format
func (g *Gemini) transformToolChoice(choice *types.ToolChoice) map[string]any {
	if choice == nil {
		return nil
	}

	switch choice.Type {
	case types.ToolChoiceTypeAuto:
		return map[string]any{
			"functionCallingConfig": map[string]any{
				"mode": "AUTO",
			},
		}
	case types.ToolChoiceTypeNone:
		return map[string]any{
			"functionCallingConfig": map[string]any{
				"mode": "NONE",
			},
		}
	case types.ToolChoiceTypeAny:
		return map[string]any{
			"functionCallingConfig": map[string]any{
				"mode": "ANY",
			},
		}
	case types.ToolChoiceTypeSpecific:
		return map[string]any{
			"functionCallingConfig": map[string]any{
				"mode":                 "ANY",
				"allowedFunctionNames": []string{choice.ToolName},
			},
		}
	default:
		return nil
	}
}

// transformSchema converts a types.Schema to Gemini schema format
func (g *Gemini) transformSchema(schema types.Schema) map[string]any {
	return g.schemaToMap(schema)
}
