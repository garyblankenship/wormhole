package openai

import (
	"context"
	"encoding/json"

	"github.com/garyblankenship/wormhole/v3/internal/pool"
	"github.com/garyblankenship/wormhole/v3/internal/structured"
	"github.com/garyblankenship/wormhole/v3/types"
)

// Structured generates a structured response
func (p *Provider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	textRequest, err := structured.Prepare(request)
	if err != nil {
		return nil, err
	}
	if request.Mode == types.StructuredModeStrict {
		schema, err := strictSchemaToMap(request.Schema)
		if err != nil {
			return nil, err
		}
		format := textRequest.ResponseFormat.(map[string]any)
		format["json_schema"].(map[string]any)["schema"] = schema
	}
	response, err := p.Text(ctx, textRequest)
	if err != nil {
		return nil, err
	}
	return structured.Convert(request, response)
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
