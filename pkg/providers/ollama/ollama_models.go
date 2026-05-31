package ollama

import (
	"context"
	"fmt"
	"net/http"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ListModels lists available Ollama models
func (p *Provider) ListModels(ctx context.Context) (*modelsResponse, error) {
	url := p.GetBaseURL() + "/api/tags"

	var response modelsResponse
	err := p.DoRequest(ctx, http.MethodGet, url, nil, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// PullModel pulls a model from Ollama registry
func (p *Provider) PullModel(ctx context.Context, model string) error {
	payload := map[string]any{
		"name": model,
	}

	url := p.GetBaseURL() + "/api/pull"

	// This is a streaming endpoint but we'll treat it as regular request for simplicity
	var response map[string]any // Ollama returns various status messages
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return p.WrapError(types.ErrorCodeProvider, fmt.Sprintf("failed to pull model %s", model), err)
	}

	return nil
}

// ShowModel shows information about a model
func (p *Provider) ShowModel(ctx context.Context, model string) (map[string]any, error) {
	payload := map[string]any{
		"name": model,
	}

	url := p.GetBaseURL() + "/api/show"

	var response map[string]any
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, p.WrapError(types.ErrorCodeProvider, fmt.Sprintf("failed to show model %s", model), err)
	}

	return response, nil
}

// DeleteModel deletes a model from Ollama
func (p *Provider) DeleteModel(ctx context.Context, model string) error {
	payload := map[string]any{
		"name": model,
	}

	url := p.GetBaseURL() + "/api/delete"

	var response map[string]any
	err := p.DoRequest(ctx, http.MethodDelete, url, payload, &response)
	if err != nil {
		return p.WrapError(types.ErrorCodeProvider, fmt.Sprintf("failed to delete model %s", model), err)
	}

	return nil
}
