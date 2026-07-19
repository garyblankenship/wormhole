package openai

import (
	"context"
	"encoding/json"

	"github.com/garyblankenship/wormhole/v2/internal/pool"
	"github.com/garyblankenship/wormhole/v2/types"
)

// Structured generates a structured response
func (p *Provider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	// Convert to text request with JSON mode or function calling
	textRequest := types.TextRequest{
		BaseRequest:  request.BaseRequest,
		Messages:     request.Messages,
		SystemPrompt: request.SystemPrompt,
	}

	// Determine the best method for structured output
	switch request.Mode {
	case types.StructuredModeJSON:
		textRequest.ResponseFormat = map[string]string{"type": "json_object"}
	case types.StructuredModeStrict:
		// Native OpenAI strict structured output: emit a json_schema response_format.
		// This is the Chat Completions (nested) shape; buildResponsesPayload reshapes
		// it to the flattened Responses API shape when that transport is active.
		schemaMap, err := schemaToMap(request.Schema)
		if err != nil {
			return nil, err
		}
		name := request.SchemaName
		if name == "" {
			name = "structured_output"
		}
		textRequest.ResponseFormat = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   name,
				"strict": true,
				"schema": schemaMap,
			},
		}
	default:
		// Use function calling for structured output
		tool, err := p.schemaToTool(request.Schema, request.SchemaName)
		if err != nil {
			return nil, err
		}
		textRequest.Tools = []types.Tool{*tool}
		textRequest.ToolChoice = &types.ToolChoice{
			Type:     types.ToolChoiceTypeSpecific,
			ToolName: tool.Name,
		}
	}

	response, err := p.Text(ctx, textRequest)
	if err != nil {
		return nil, err
	}

	data, err := p.extractStructuredData(request.Mode, response)
	if err != nil {
		return nil, err
	}

	return &types.StructuredResponse{
		ID:      response.ID,
		Model:   response.Model,
		Data:    data,
		Usage:   response.Usage,
		Created: response.Created,
	}, nil
}

// extractStructuredData decodes the model response into structured data per the
// requested mode: JSON/strict modes unmarshal response text; otherwise the first
// tool call's arguments. Returns an already-wrapped error on failure.
func (p *Provider) extractStructuredData(mode types.StructuredMode, response *types.TextResponse) (any, error) {
	var data any
	var err error
	switch {
	case mode == types.StructuredModeJSON || mode == types.StructuredModeStrict:
		err = json.Unmarshal([]byte(response.Text), &data)
	case len(response.ToolCalls) > 0:
		argsBytes, marshalErr := pool.Marshal(response.ToolCalls[0].Arguments)
		if marshalErr != nil {
			err = marshalErr
		} else {
			defer pool.Return(argsBytes)
			err = json.Unmarshal(argsBytes, &data)
		}
	default:
		err = p.ProviderError("no structured data in response")
	}
	if err != nil {
		return nil, p.RequestError("failed to parse structured response", err)
	}
	return data, nil
}
