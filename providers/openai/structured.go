package openai

import (
	"context"

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
