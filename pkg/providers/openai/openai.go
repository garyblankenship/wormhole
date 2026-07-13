package openai

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/garyblankenship/wormhole/internal/pool"
	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/providers"
	transform "github.com/garyblankenship/wormhole/pkg/providers/transform"
	"github.com/garyblankenship/wormhole/pkg/types"
)

const (
	defaultBaseURL = "https://api.openai.com/v1"
)

// Provider implements the OpenAI provider
type Provider struct {
	*providers.BaseProvider
	requestBuilder       *providers.RequestBuilder
	responseTransform    *transform.ResponseTransform
	streamingTransformer *transform.StreamingTransformer
}

var _ types.Provider = (*Provider)(nil)

// New creates a new OpenAI provider
func New(config types.ProviderConfig) *Provider {
	return NewWithName("openai", config)
}

// NewWithName creates an OpenAI-compatible provider with a caller-visible provider name.
func NewWithName(name string, config types.ProviderConfig) *Provider {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}

	return &Provider{
		BaseProvider:         providers.NewBaseProvider(name, config),
		requestBuilder:       providers.NewRequestBuilder(),
		responseTransform:    transform.NewResponseTransform(),
		streamingTransformer: transform.NewOpenAIStreamingTransformer(),
	}
}

// chatCompletionsURL returns the chat-completions endpoint, honoring a
// configured ChatPath override (empty = the OpenAI default).
func (p *Provider) chatCompletionsURL() string {
	path := p.Config.ChatPath
	if path == "" {
		path = "/chat/completions"
	}
	return p.GetBaseURL() + path
}

// responsesURL returns the Responses API endpoint, honoring a configured
// ResponsesPath override (empty = the OpenAI default).
func (p *Provider) responsesURL() string {
	path := p.Config.ResponsesPath
	if path == "" {
		path = "/responses"
	}
	return p.GetBaseURL() + path
}

// imagesURL returns the image-generation endpoint, honoring a configured
// ImagePath override (empty = the OpenAI default).
func (p *Provider) imagesURL() string {
	path := p.Config.ImagePath
	if path == "" {
		path = "/images/generations"
	}
	return p.GetBaseURL() + path
}

// SupportedCapabilities returns the capabilities supported by OpenAI provider
func (p *Provider) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{
		types.CapabilityText,
		types.CapabilityChat,
		types.CapabilityStructured,
		types.CapabilityEmbeddings,
		types.CapabilityAudio,
		types.CapabilityImages,
		types.CapabilityStream,
		types.CapabilityFunctions,
	}
}

// Text generates a text response
func (p *Provider) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	if _, _, err := providers.PrepareMessages(request.Messages); err != nil {
		return nil, err
	}
	if p.Config.UseResponsesAPI {
		return p.responsesText(ctx, request)
	}

	payload := p.buildChatPayload(&request)

	url := p.chatCompletionsURL()

	var response chatCompletionResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	textResponse := p.transformTextResponse(&response)
	textResponse.Provider = p.Name()

	// Validate response has content to prevent silent failures
	if textResponse.Text == "" && len(textResponse.ToolCalls) == 0 {
		return nil, p.ProviderError("received empty response from OpenAI API", "no content or tool calls returned")
	}

	return textResponse, nil
}

// Stream generates a streaming text response
func (p *Provider) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	if _, _, err := providers.PrepareMessages(request.Messages); err != nil {
		return nil, err
	}
	if p.Config.UseResponsesAPI {
		return p.responsesStream(ctx, request)
	}

	payload := p.buildChatPayload(&request)
	payload["stream"] = true
	// Ask OpenAI to emit a final usage-bearing chunk on streamed responses;
	// without this, streamed Usage is always nil.
	payload["stream_options"] = map[string]any{"include_usage": true}

	url := p.chatCompletionsURL()

	body, err := p.StreamRequest(ctx, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}

	return p.stampProvider(ctx, p.accumulatingStream(ctx, utils.ProcessStream(ctx, body, p.parseStreamChunk, 100))), nil
}

// stampProvider sets Provider on the terminal chunk. Sole closer of out;
// exits when the upstream channel closes.
func (p *Provider) stampProvider(ctx context.Context, in <-chan types.TextChunk) <-chan types.TextChunk {
	out := make(chan types.TextChunk)
	go func() {
		defer close(out)
		for chunk := range in {
			if chunk.IsDone() {
				chunk.Provider = p.Name()
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

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

// Embeddings generates embeddings
func (p *Provider) Embeddings(ctx context.Context, request types.EmbeddingsRequest) (*types.EmbeddingsResponse, error) {
	payload := map[string]any{
		"model": request.Model,
		"input": request.Input,
	}

	if request.Dimensions != nil {
		payload["dimensions"] = *request.Dimensions
	}

	// Merge provider-specific options (allows overriding any parameter)
	for k, v := range p.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[k] = v
	}

	url := p.GetBaseURL() + "/embeddings"

	var response embeddingsResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	resp := p.transformEmbeddingsResponse(&response, request.Model)
	resp.Provider = p.Name()
	return resp, nil
}

// Rerank reranks documents by relevance to a query (OpenAI-compatible /rerank).
func (p *Provider) Rerank(ctx context.Context, request types.RerankRequest) (*types.RerankResponse, error) {
	payload := map[string]any{
		"model":     request.Model,
		"query":     request.Query,
		"documents": request.Documents,
	}

	if request.TopN != nil {
		payload["top_n"] = *request.TopN
	}

	// Merge provider-specific options (allows overriding any parameter)
	for k, v := range p.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[k] = v
	}

	url := p.GetBaseURL() + "/rerank"

	var response rerankResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	resp := p.transformRerankResponse(&response, request.Model)
	resp.Provider = p.Name()
	return resp, nil
}

// Images generates images
func (p *Provider) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	payload := map[string]any{
		"model":  request.Model,
		"prompt": request.Prompt,
	}

	if request.Size != "" {
		payload["size"] = request.Size
	}
	if request.Quality != "" {
		payload["quality"] = request.Quality
	}
	if request.Style != "" {
		payload["style"] = request.Style
	}
	if request.N > 0 {
		payload["n"] = request.N
	}
	if request.ResponseFormat != "" {
		payload["response_format"] = request.ResponseFormat
	}

	// Merge provider-specific options (allows overriding any parameter)
	for k, v := range p.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[k] = v
	}

	url := p.imagesURL()

	var response imageResponse
	err := p.DoRequest(ctx, http.MethodPost, url, payload, &response)
	if err != nil {
		return nil, err
	}

	return p.transformImageResponse(&response), nil
}

// GenerateImage generates images through the unified image-generation interface.
func (p *Provider) GenerateImage(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
	return p.Images(ctx, request)
}

// Temporarily disabled until request types are defined
// These methods will be automatically provided by embedded BaseProvider with NotImplementedError
