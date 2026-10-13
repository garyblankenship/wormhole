package openai

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
)

// decisionsURL derives the sibling alpha API while preserving custom host/path.
func (p *Provider) decisionsURL() (string, error) {
	u, err := url.Parse(p.GetBaseURL())
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", p.ValidationError("invalid Decisions base URL")
	}
	path := strings.TrimRight(u.Path, "/")
	path = strings.TrimSuffix(path, "/v1")
	u.Path = path + "/alpha/decisions"
	if u.RawPath != "" {
		u.RawPath = strings.TrimSuffix(strings.TrimRight(u.RawPath, "/"), "/v1") + "/alpha/decisions"
	}
	return u.String(), nil
}

// Decisions calls OpenRouter's direct Jev alpha operation, never chat completions.
func (p *Provider) Decisions(ctx context.Context, request types.DecisionsRequest) (*types.DecisionsResponse, error) {
	if p.Name() != "openrouter" {
		return nil, p.NotImplementedError("Decisions")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	endpoint, err := p.decisionsURL()
	if err != nil {
		return nil, err
	}
	var response types.DecisionsResponse
	if err := p.DoRequest(ctx, http.MethodPost, endpoint, request, &response); err != nil {
		return nil, err
	}
	if err := response.ValidateFor(request); err != nil {
		return nil, p.ProviderError("invalid Decisions response", err.Error())
	}
	response.Provider = p.Name()
	if response.Model == "" {
		response.Model = request.Model
	}
	return &response, nil
}
