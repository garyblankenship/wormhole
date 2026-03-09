package openai

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/internal/utils"
	providerTransform "github.com/garyblankenship/wormhole/pkg/providers/transform"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// Tool choice options
const toolChoiceAuto = "auto"

// buildChatPayload builds the OpenAI chat completion payload
func (p *Provider) buildChatPayload(request *types.TextRequest) map[string]any {
	payload := map[string]any{
		"model":    request.Model,
		"messages": p.transformMessages(request.Messages),
	}

	// Add generation parameters
	p.addGenerationParams(payload, request)

	// Add tools if present
	p.addToolsParams(payload, request)

	// Add response format if specified
	if request.ResponseFormat != nil {
		payload["response_format"] = request.ResponseFormat
	}

	// Merge provider-specific options (allows overriding any parameter)
	for k, v := range request.ProviderOptions {
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
		if paramName != "max_tokens" {
			// Remove the generic max_tokens added by shared utility
			delete(payload, "max_tokens")
			payload[paramName] = *request.MaxTokens
		}
	}
}

// getMaxTokensParam returns the appropriate max tokens parameter name for the model
func (p *Provider) getMaxTokensParam(model string) string {
	// Check for provider-specific parameter configuration
	if p.Config.Params != nil {
		if param, ok := p.Config.Params["max_tokens_param"].(string); ok {
			return param
		}
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
// Delegates to shared utility for consistent behavior across providers
func cleanJSONResponse(content string) string {
	return utils.ExtractJSONFromMarkdown(content)
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

	// Clean JSON responses that may be wrapped in markdown code blocks
	// This is particularly needed for Anthropic models via OpenRouter that return JSON in code blocks
	if strings.Contains(content, "```") &&
		(strings.Contains(strings.ToLower(response.Model), "claude") ||
			strings.Contains(strings.ToLower(response.Model), "anthropic")) {
		content = cleanJSONResponse(content)
	}

	return &types.TextResponse{
		ID:           response.ID,
		Model:        response.Model,
		Text:         content,
		ToolCalls:    p.convertToolCalls(choice.Message.ToolCalls),
		FinishReason: p.mapFinishReason(choice.FinishReason),
		Usage:        p.convertUsage(response.Usage),
		Created:      time.Unix(response.Created, 0),
	}
}

// transformEmbeddingsResponse converts OpenAI embeddings response
func (p *Provider) transformEmbeddingsResponse(response *embeddingsResponse) *types.EmbeddingsResponse {
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

	return &types.EmbeddingsResponse{
		Model:      response.Model,
		Embeddings: embeddings,
		Usage:      p.convertUsage(response.Usage),
		Created:    time.Now(),
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
		// Parse arguments from JSON string to map[string]any
		var argsMap map[string]any
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &argsMap); err != nil {
				// If parsing fails, create empty map
				argsMap = make(map[string]any)
			}
		} else {
			argsMap = make(map[string]any)
		}

		result[i] = types.ToolCall{
			ID:        tc.ID,
			Type:      tc.Type,
			Name:      tc.Function.Name, // Set top-level Name field
			Arguments: argsMap,          // Set top-level Arguments field
			Function: &types.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}

	return result
}

func (p *Provider) convertUsage(u usage) *types.Usage {
	return &types.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
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

func (p *Provider) schemaToTool(schema types.Schema, name string) (*types.Tool, error) {
	if name == "" {
		name = "structured_output"
	}

	// Convert Schema interface to map[string]any
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var params map[string]any
	if err := json.Unmarshal(schemaBytes, &params); err != nil {
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
