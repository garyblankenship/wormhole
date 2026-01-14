package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/providers"
	"github.com/garyblankenship/wormhole/pkg/types"
)

const (
	defaultBaseURL   = "https://api.anthropic.com/v1"
	anthropicVersion = "2023-06-01"
	headerAPIKey     = "x-api-key"
	headerVersion    = "anthropic-version"
)

// Provider implements the Anthropic provider
type Provider struct {
	*providers.BaseProvider
}

// New creates a new Anthropic provider
func New(config types.ProviderConfig) *Provider {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}

	// Add Anthropic-specific headers
	if config.Headers == nil {
		config.Headers = make(map[string]string)
	}
	config.Headers[headerVersion] = anthropicVersion
	config.Headers[headerAPIKey] = config.APIKey

	return &Provider{
		BaseProvider: providers.NewBaseProvider("anthropic", config),
	}
}

// Text generates a text response
func (p *Provider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	payload := p.buildMessagePayload(&request)

	url := p.GetBaseURL() + "/messages"

	var response messageResponse
	err := p.doAnthropicRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	return p.transformTextResponse(&response), nil
}

// Stream generates a streaming text response
func (p *Provider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.StreamChunk, error) {
	payload := p.buildMessagePayload(&request)
	payload["stream"] = true

	url := p.GetBaseURL() + "/messages"

	body, err := p.streamAnthropicRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	return utils.ProcessStream(body, p.parseStreamChunk, 100), nil
}

// Structured generates a structured response
func (p *Provider) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	// Anthropic uses tool calling for structured output
	textRequest := types.TextRequest{
		BaseRequest:  request.BaseRequest,
		Messages:     request.Messages,
		SystemPrompt: request.SystemPrompt,
	}

	// Create a tool from the schema
	schemaBytes, err := json.Marshal(request.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}
	tool, err := p.schemaToTool(json.RawMessage(schemaBytes), request.SchemaName)
	if err != nil {
		return nil, err
	}

	textRequest.Tools = []types.Tool{*tool}
	textRequest.ToolChoice = &types.ToolChoice{
		Type:     types.ToolChoiceTypeSpecific,
		ToolName: tool.Name,
	}

	response, err := p.Text(ctx, textRequest)
	if err != nil {
		return nil, err
	}

	// Extract structured data from tool call
	if len(response.ToolCalls) == 0 {
		return nil, fmt.Errorf("no tool call in response")
	}

	var data any
	if response.ToolCalls[0].Function != nil {
		err = utils.UnmarshalAnthropicToolArgs(response.ToolCalls[0].Function.Arguments, &data)
	} else {
		// Fallback to Arguments field
		jsonBytes, _ := json.Marshal(response.ToolCalls[0].Arguments)
		err = utils.LenientUnmarshal(jsonBytes, &data)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse structured response: %w", err)
	}

	return &types.StructuredResponse{
		ID:      response.ID,
		Model:   response.Model,
		Data:    data,
		Usage:   response.Usage,
		Created: response.Created,
	}, nil
}

// Embeddings is not supported by Anthropic
func (p *Provider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	return nil, p.NotImplementedError("Embeddings")
}

// Audio is not supported by Anthropic
func (p *Provider) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	return nil, p.NotImplementedError("Audio")
}

// Images is not supported by Anthropic
func (p *Provider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	return nil, p.NotImplementedError("Images")
}

// doAnthropicRequest performs an HTTP request with Anthropic-specific headers
func (p *Provider) doAnthropicRequest(ctx context.Context, method, url string, body any, result any) error {
	// Use custom header handling for Anthropic
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set(types.HeaderContentType, types.ContentTypeJSON)
	req.Header.Set(headerAPIKey, p.Config.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	for k, v := range p.Config.Headers {
		if k != headerAPIKey && k != headerVersion {
			req.Header.Set(k, v)
		}
	}

	resp, err := p.GetHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("warning: failed to close response body: %v", err)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiError anthropicError
		if err := json.Unmarshal(respBody, &apiError); err != nil {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
		}
		return apiError
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// streamAnthropicRequest performs a streaming HTTP request
func (p *Provider) streamAnthropicRequest(ctx context.Context, method, url string, body any) (io.ReadCloser, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set(types.HeaderContentType, types.ContentTypeJSON)
	req.Header.Set(headerAPIKey, p.Config.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set(types.HeaderAccept, types.ContentTypeEventStream)
	req.Header.Set(types.HeaderCacheControl, "no-cache")

	for k, v := range p.Config.Headers {
		if k != headerAPIKey && k != headerVersion {
			req.Header.Set(k, v)
		}
	}

	resp, err := p.GetHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("warning: failed to close response body: %v", err)
		}
	}()
		respBody, _ := io.ReadAll(resp.Body)
		var apiError anthropicError
		if err := json.Unmarshal(respBody, &apiError); err != nil {
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
		}
		return nil, apiError
	}

	return resp.Body, nil
}
