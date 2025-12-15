package openai

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/internal/utils"
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
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		payload[p.getMaxTokensParam(request.Model)] = *request.MaxTokens
	}
	if len(request.Stop) > 0 {
		payload["stop"] = request.Stop
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
		openAIMsg := map[string]any{
			"role": string(msg.GetRole()),
		}

		// Handle content based on type
		content := msg.GetContent()
		switch c := content.(type) {
		case string:
			openAIMsg["content"] = c
		case []types.MessagePart:
			// Multi-modal content
			parts := make([]map[string]any, len(c))
			for j, part := range c {
				if part.Type == "text" {
					parts[j] = map[string]any{
						"type": "text",
						"text": part.Text,
					}
				} else if part.Type == "image" {
					parts[j] = map[string]any{
						"type":      "image_url",
						"image_url": part.Data,
					}
				}
			}
			openAIMsg["content"] = parts
		default:
			// Try to convert to string
			openAIMsg["content"] = fmt.Sprintf("%v", content)
		}

		// Handle assistant messages with tool calls
		if assistantMsg, ok := msg.(*types.AssistantMessage); ok && len(assistantMsg.ToolCalls) > 0 {
			openAIMsg["tool_calls"] = p.transformToolCalls(assistantMsg.ToolCalls)
		}

		// Handle tool messages
		if toolMsg, ok := msg.(*types.ToolMessage); ok {
			openAIMsg["tool_call_id"] = toolMsg.ToolCallID
		}

		result[i] = openAIMsg
	}

	return result
}

// transformTools converts internal tools to OpenAI format
func (p *Provider) transformTools(tools []types.Tool) []map[string]any {
	result := make([]map[string]any, len(tools))

	for i, tool := range tools {
		parameters, _ := json.Marshal(tool.Function.Parameters)
		result[i] = map[string]any{
			"type": tool.Type,
			"function": map[string]any{
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
				"parameters":  json.RawMessage(parameters),
			},
		}
	}

	return result
}

// transformToolCalls converts internal tool calls to OpenAI format
func (p *Provider) transformToolCalls(toolCalls []types.ToolCall) []map[string]any {
	result := make([]map[string]any, len(toolCalls))

	for i, tc := range toolCalls {
		result[i] = map[string]any{
			"id":   tc.ID,
			"type": tc.Type,
			"function": map[string]any{
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
			},
		}
	}

	return result
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
	switch reason {
	case "stop":
		return types.FinishReasonStop
	case "length":
		return types.FinishReasonLength
	case "tool_calls", "function_call":
		return types.FinishReasonToolCalls
	case "content_filter":
		return types.FinishReasonContentFilter
	default:
		return types.FinishReasonStop
	}
}

// transformToolChoice converts tool choice to OpenAI format
func (p *Provider) transformToolChoice(choice *types.ToolChoice) any {
	if choice == nil {
		return toolChoiceAuto
	}

	switch choice.Type {
	case types.ToolChoiceTypeNone:
		return "none"
	case types.ToolChoiceTypeAuto:
		return toolChoiceAuto
	case types.ToolChoiceTypeAny:
		return "required"
	case types.ToolChoiceTypeSpecific:
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": choice.ToolName,
			},
		}
	default:
		return toolChoiceAuto
	}
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
