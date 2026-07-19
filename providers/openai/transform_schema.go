package openai

import (
	"encoding/json"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

// transformToolChoice converts tool choice to OpenAI format
func (p *Provider) transformToolChoice(choice *types.ToolChoice) any {
	// Use shared RequestBuilder for common tool choice transformation
	sharedResult := p.requestBuilder.TransformToolChoice(choice)

	// Handle OpenAI-specific ToolChoiceTypeAny
	if choice != nil && choice.Type == types.ToolChoiceTypeAny {
		return "required"
	}

	// Return shared result (handles nil, None, Auto, Specific)
	// If sharedResult is nil (choice is nil), return default "auto"
	if sharedResult == nil {
		return toolChoiceAuto
	}
	return sharedResult
}

// schemaToMap converts a Schema (any) into a map[string]any via JSON round-trip.
// Single source of truth for schema->wire-map, reused by structured-output paths.
func schemaToMap(schema types.Schema) (map[string]any, error) {
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(schemaBytes, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (p *Provider) schemaToTool(schema types.Schema, name string) (*types.Tool, error) {
	if name == "" {
		name = "structured_output"
	}

	params, err := schemaToMap(schema)
	if err != nil {
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

// isGPT5Model determines if a model requires GPT-5 API parameters
func isGPT5Model(model string) bool {
	// Check if model contains "gpt-5" anywhere in the name (case-insensitive)
	// Handles: gpt-5, gpt-5-mini, openai/gpt-5-mini, etc.
	model = strings.ToLower(model)
	return strings.Contains(model, "gpt-5")
}
