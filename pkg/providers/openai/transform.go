package openai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/pkg/providers"
	providerTransform "github.com/garyblankenship/wormhole/pkg/providers/transform" //nolint:staticcheck // Supported v1 implementation dependency; internalized in v2.
	"github.com/garyblankenship/wormhole/pkg/types"
)

// Tool choice options
const toolChoiceAuto = "auto"

// buildChatPayload builds the OpenAI chat completion payload
func (p *Provider) buildChatPayload(request *types.TextRequest) map[string]any {
	prepared, _, err := providers.PrepareMessages(request.Messages)
	if err != nil {
		prepared = request.Messages // fall through; provider will surface the issue
	}
	payload := map[string]any{
		"model":    request.Model,
		"messages": p.transformMessages(prepared),
	}

	// Add generation parameters
	p.addGenerationParams(payload, request)

	p.addReasoningParams(payload, request)

	// Add tools if present
	p.addToolsParams(payload, request)

	// Add response format if specified
	if request.ResponseFormat != nil {
		payload["response_format"] = request.ResponseFormat
	}

	// Merge provider-specific options (allows overriding any parameter)
	for k, v := range p.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[k] = v
	}

	return payload
}

// addGenerationParams adds temperature, top_p, max_tokens, and stop sequences to payload
func (p *Provider) addGenerationParams(payload map[string]any, request *types.TextRequest) {
	// Use shared utility for common parameters
	p.requestBuilder.AddGenerationParams(payload, request.Temperature, request.TopP, request.MaxTokens, request.Stop)

	// OpenAI-specific: adjust max tokens parameter name for GPT-5 models
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		paramName := p.getMaxTokensParam(request.Model)
		maxTokens := p.maxTokensValue(*request.MaxTokens)
		if paramName != "max_tokens" {
			// Remove the generic max_tokens added by shared utility
			delete(payload, "max_tokens")
			payload[paramName] = maxTokens
		} else {
			payload[paramName] = maxTokens
		}
	}
}

func (p *Provider) addReasoningParams(payload map[string]any, request *types.TextRequest) {
	if reasoning := reasoningPayload(request.Reasoning); len(reasoning) > 0 {
		payload["reasoning"] = reasoning
	}
}

func reasoningPayload(reasoning *types.Reasoning) map[string]any {
	if reasoning == nil {
		return nil
	}
	out := make(map[string]any, 3)
	if reasoning.Effort != "" && reasoning.Effort != types.ReasoningEffortNone {
		out["effort"] = string(reasoning.Effort)
	}
	if reasoning.MaxTokens > 0 {
		out["max_tokens"] = reasoning.MaxTokens
	}
	if reasoning.Enabled != nil {
		out["enabled"] = *reasoning.Enabled
	}
	return out
}

func (p *Provider) maxTokensValue(value int) int {
	if cap := p.Config.RequestPolicy.MaxTokensCap; cap > 0 && value > cap {
		return cap
	}
	return value
}

// getMaxTokensParam returns the appropriate max tokens parameter name for the model
func (p *Provider) getMaxTokensParam(model string) string {
	// Check for provider-specific parameter configuration
	if p.Config.Params != nil {
		if param, ok := p.Config.Params["max_tokens_param"].(string); ok {
			return param
		}
	}
	for _, rule := range p.Config.RequestPolicy.MaxTokensParamRules {
		if rule.ModelContains != "" && rule.Param != "" && strings.Contains(strings.ToLower(model), strings.ToLower(rule.ModelContains)) {
			return rule.Param
		}
	}
	if p.Config.RequestPolicy.MaxTokensParam != "" {
		return p.Config.RequestPolicy.MaxTokensParam
	}
	// GPT-5 models require max_completion_tokens instead of deprecated max_tokens
	if isGPT5Model(model) {
		return "max_completion_tokens"
	}
	return "max_tokens"
}

// addToolsParams adds tools and tool_choice to payload if tools are present
func (p *Provider) addToolsParams(payload map[string]any, request *types.TextRequest) {
	if len(request.Tools) == 0 {
		return
	}
	payload["tools"] = p.transformTools(request.Tools)
	if request.ToolChoice != nil {
		payload["tool_choice"] = p.transformToolChoice(request.ToolChoice)
	}
}

// transformMessages converts internal messages to OpenAI format
func (p *Provider) transformMessages(messages []types.Message) []map[string]any {
	result := make([]map[string]any, len(messages))

	for i, msg := range messages {
		// Use shared RequestBuilder for basic message transformation
		// This handles role, content, tool calls, and tool call IDs
		openAIMsg := p.requestBuilder.TransformMessage(msg)

		if userMsg, ok := msg.(*types.UserMessage); ok && len(userMsg.Media) > 0 {
			openAIMsg["content"] = p.transformUserMessageContent(userMsg)
		}

		// Transform content if it's multi-modal ([]types.MessagePart)
		// OpenAI requires specific format for multi-modal content
		if content, ok := openAIMsg["content"].([]types.MessagePart); ok {
			parts := make([]map[string]any, len(content))
			for j, part := range content {
				switch part.Type {
				case "text":
					parts[j] = map[string]any{
						"type": "text",
						"text": part.Text,
					}
				case "image":
					parts[j] = map[string]any{
						"type":      "image_url",
						"image_url": part.Data,
					}
				}
			}
			openAIMsg["content"] = parts
		}

		result[i] = openAIMsg
	}

	return result
}

func (p *Provider) transformUserMessageContent(msg *types.UserMessage) any {
	parts := make([]map[string]any, 0, 1+len(msg.Media))
	if msg.Content != "" {
		parts = append(parts, map[string]any{
			"type": "text",
			"text": msg.Content,
		})
	}

	for _, media := range msg.Media {
		if image, ok := media.(*types.ImageMedia); ok {
			url, ok := imageMediaURL(image)
			if !ok {
				continue
			}
			parts = append(parts, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": url,
				},
			})
		}
	}

	return parts
}

func imageMediaURL(image *types.ImageMedia) (string, bool) {
	if image.URL != "" {
		return image.URL, true
	}
	data := image.Base64Data
	if data == "" && len(image.Data) > 0 {
		data = base64.StdEncoding.EncodeToString(image.Data)
	}
	if data == "" {
		return "", false
	}
	mimeType := image.MimeType
	if mimeType == "" {
		mimeType = "image/png"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, data), true
}

// transformTools converts internal tools to OpenAI format
func (p *Provider) transformTools(tools []types.Tool) []map[string]any {
	// Use shared RequestBuilder for common tool transformation
	baseTools := p.requestBuilder.TransformTools(tools)

	// Adapt to OpenAI-specific format (json.RawMessage for parameters)
	for _, baseTool := range baseTools {
		if toolFunc, ok := baseTool["function"].(map[string]any); ok {
			if params, ok := toolFunc["parameters"].(map[string]any); ok {
				// Convert map to json.RawMessage
				parameters, _ := json.Marshal(params)
				toolFunc["parameters"] = json.RawMessage(parameters)
			}
		}
		// Ensure type field is present (OpenAI requires "type": "function")
		if _, hasType := baseTool["type"]; !hasType {
			baseTool["type"] = "function"
		}
	}

	return baseTools
}

// cleanJSONResponse removes markdown code blocks from JSON responses
func cleanJSONResponse(content string) string {
	return extractJSONFromMarkdown(content)
}

// transformTextResponse converts OpenAI response to internal format
func (p *Provider) transformTextResponse(response *chatCompletionResponse) *types.TextResponse {
	if len(response.Choices) == 0 {
		return &types.TextResponse{
			ID:      response.ID,
			Model:   response.Model,
			Created: time.Unix(response.Created, 0),
		}
	}

	choice := response.Choices[0]
	content := choice.Message.Content

	// Strip markdown code fences from JSON responses regardless of model.
	// cleanJSONResponse is a no-op when there are no backticks and only
	// strips when the extracted content is valid-looking JSON, so this is
	// safe for every provider/model and avoids brittle model-name sniffing.
	content = cleanJSONResponse(content)

	resp := &types.TextResponse{
		ID:           response.ID,
		Model:        response.Model,
		Text:         content,
		Refusal:      choice.Message.Refusal,
		ToolCalls:    p.convertToolCalls(choice.Message.ToolCalls),
		FinishReason: p.mapFinishReason(choice.FinishReason),
		Usage:        p.convertUsage(response.Usage),
		Created:      time.Unix(response.Created, 0),
	}

	if choice.Message.ReasoningContent != "" {
		resp.Thinking = &types.Thinking{Content: choice.Message.ReasoningContent}
	}

	return resp
}

// transformEmbeddingsResponse converts OpenAI embeddings response
func (p *Provider) transformEmbeddingsResponse(response *embeddingsResponse, requestModel string) *types.EmbeddingsResponse {
	embeddings := make([]types.Embedding, len(response.Data))

	for i, data := range response.Data {
		// Convert []float32 to []float64
		embedding := make([]float64, len(data.Embedding))
		for j, v := range data.Embedding {
			embedding[j] = float64(v)
		}
		embeddings[i] = types.Embedding{
			Index:     data.Index,
			Embedding: embedding,
		}
	}

	model := response.Model
	if model == "" {
		model = requestModel
	}

	return &types.EmbeddingsResponse{
		Model:      model,
		Embeddings: embeddings,
		Usage:      p.convertUsage(response.Usage),
		Created:    time.Now(),
	}
}

// transformRerankResponse converts an OpenAI-compatible rerank response.
func (p *Provider) transformRerankResponse(response *rerankResponse, requestModel string) *types.RerankResponse {
	results := make([]types.RerankResult, len(response.Results))
	for i, r := range response.Results {
		results[i] = types.RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
			Document:       r.Document.Text,
		}
	}

	model := response.Model
	if model == "" {
		model = requestModel
	}

	return &types.RerankResponse{
		ID:      response.ID,
		Model:   model,
		Results: results,
		Usage:   &types.Usage{TotalTokens: response.Usage.TotalTokens},
		Created: time.Now(),
	}
}

// transformImageResponse converts OpenAI image response
func (p *Provider) transformImageResponse(response *imageResponse) *types.ImagesResponse {
	images := make([]types.GeneratedImage, len(response.Data))

	for i, data := range response.Data {
		images[i] = types.GeneratedImage{
			URL:     data.URL,
			B64JSON: data.B64JSON,
		}
	}

	return &types.ImagesResponse{
		Images:  images,
		Created: time.Unix(response.Created, 0),
	}
}

// parseStreamChunk parses a streaming chunk
func (p *Provider) parseStreamChunk(data []byte) (*types.TextChunk, error) {
	// Try to use unified streaming transformer if available
	if p.streamingTransformer != nil {
		return p.streamingTransformer.ParseChunk(data)
	}

	// Fall back to original implementation
	var response streamResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	if len(response.Choices) == 0 {
		return nil, nil
	}

	choice := response.Choices[0]
	chunk := &types.StreamChunk{
		ID:    response.ID,
		Model: response.Model,
		Text:  choice.Delta.Content, // Set Text field for backward compatibility
		Delta: &types.ChunkDelta{
			Content: choice.Delta.Content,
		},
	}

	if choice.Delta.Refusal != "" {
		chunk.Refusal = choice.Delta.Refusal
		chunk.Delta.Refusal = choice.Delta.Refusal
	}

	if choice.Delta.ReasoningContent != "" {
		thinking := &types.Thinking{Content: choice.Delta.ReasoningContent}
		chunk.Thinking = thinking
		chunk.Delta.Thinking = thinking
	}

	if len(choice.Delta.ToolCalls) > 0 {
		chunk.ToolCalls = p.convertToolCalls(choice.Delta.ToolCalls)
	}

	if choice.FinishReason != "" {
		reason := p.mapFinishReason(choice.FinishReason)
		chunk.FinishReason = &reason
	}

	if response.Usage != nil {
		chunk.Usage = p.convertUsage(*response.Usage)
	}

	return chunk, nil
}

// Helper functions

func (p *Provider) convertToolCalls(toolCalls []toolCall) []types.ToolCall {
	result := make([]types.ToolCall, len(toolCalls))

	for i, tc := range toolCalls {
		// Parse arguments from JSON string to map[string]any. For streaming
		// fragments tc.Function.Arguments is partial JSON that will not parse;
		// the accumulator (stream_accumulator.go) stitches fragments by index
		// and parses once. We always carry the raw fragment string in
		// Function.Arguments so the accumulator can reassemble it.
		// Empty default nil: an absent arg set stays a nil map here (the streaming
		// accumulator carries the raw fragment in Function.Arguments and reparses).
		argsMap, parseErrMsg := types.ParseToolArgs(tc.Function.Arguments, nil)

		toolCall := types.ToolCall{
			Index:     i,
			ID:        tc.ID,
			Type:      tc.Type,
			Name:      tc.Function.Name,
			Arguments: argsMap,
			Function: &types.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
		if tc.Index != nil {
			toolCall.Index = *tc.Index
		}
		toolCall.MarkArgsError(parseErrMsg)
		result[i] = toolCall
	}

	return result
}

func (p *Provider) convertUsage(u usage) *types.Usage {
	usage := &types.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
	if u.PromptTokensDetails != nil {
		usage.CacheReadTokens = u.PromptTokensDetails.CachedTokens
	}
	if usage.CacheReadTokens == 0 && u.PromptCacheHitTokens > 0 {
		usage.CacheReadTokens = u.PromptCacheHitTokens
	}
	if u.CompletionTokensDetails != nil {
		usage.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	return usage
}

func (p *Provider) mapFinishReason(reason string) types.FinishReason {
	return providerTransform.MapFinishReason(reason)
}

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
