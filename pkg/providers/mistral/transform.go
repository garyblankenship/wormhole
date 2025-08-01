package mistral

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/prism-php/prism-go/pkg/types"
)

// buildChatPayload builds the Mistral chat completion payload
func (p *Provider) buildChatPayload(request *types.TextRequest) map[string]interface{} {
	payload := map[string]interface{}{
		"model":    request.Model,
		"messages": p.transformMessages(request.Messages),
	}

	// Optional parameters
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		payload["max_tokens"] = *request.MaxTokens
	}
	if len(request.Stop) > 0 {
		payload["stop"] = request.Stop
	}

	// Tools
	if len(request.Tools) > 0 {
		payload["tools"] = p.transformTools(request.Tools)
		if request.ToolChoice != nil {
			payload["tool_choice"] = p.transformToolChoice(request.ToolChoice)
		}
	}

	// Response format for JSON mode
	if request.ResponseFormat != nil {
		payload["response_format"] = request.ResponseFormat
	}

	// Provider-specific options
	if request.ProviderOptions != nil {
		// Mistral-specific parameters
		if randomSeed, ok := request.ProviderOptions["random_seed"].(int); ok {
			payload["random_seed"] = randomSeed
		}
		if safePrompt, ok := request.ProviderOptions["safe_prompt"].(bool); ok {
			payload["safe_prompt"] = safePrompt
		}
	}

	return payload
}

// transformMessages converts internal messages to Mistral format
func (p *Provider) transformMessages(messages []types.Message) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))

	for i, msg := range messages {
		mistralMsg := map[string]interface{}{
			"role": string(msg.GetRole()),
		}

		// Handle content based on type
		content := msg.GetContent()
		switch c := content.(type) {
		case string:
			mistralMsg["content"] = c
		case []types.MessagePart:
			// Multi-modal content - Mistral supports images and documents
			parts := make([]map[string]interface{}, len(c))
			for j, part := range c {
				if part.Type == "text" {
					parts[j] = map[string]interface{}{
						"type": "text",
						"text": part.Text,
					}
				} else if part.Type == "image" {
					parts[j] = map[string]interface{}{
						"type":      "image_url",
						"image_url": part.Data,
					}
				}
			}
			mistralMsg["content"] = parts
		default:
			// Try to convert to string
			mistralMsg["content"] = fmt.Sprintf("%v", content)
		}

		// Handle assistant messages with tool calls
		if assistantMsg, ok := msg.(*types.AssistantMessage); ok && len(assistantMsg.ToolCalls) > 0 {
			mistralMsg["tool_calls"] = p.transformToolCalls(assistantMsg.ToolCalls)
		}

		// Handle tool messages
		if toolMsg, ok := msg.(*types.ToolMessage); ok {
			mistralMsg["tool_call_id"] = toolMsg.ToolCallID
		}

		result[i] = mistralMsg
	}

	return result
}

// transformTools converts internal tools to Mistral format
func (p *Provider) transformTools(tools []types.Tool) []map[string]interface{} {
	result := make([]map[string]interface{}, len(tools))

	for i, tool := range tools {
		parameters, _ := json.Marshal(tool.Function.Parameters)
		result[i] = map[string]interface{}{
			"type": tool.Type,
			"function": map[string]interface{}{
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
				"parameters":  json.RawMessage(parameters),
			},
		}
	}

	return result
}

// transformToolCalls converts internal tool calls to Mistral format
func (p *Provider) transformToolCalls(toolCalls []types.ToolCall) []map[string]interface{} {
	result := make([]map[string]interface{}, len(toolCalls))

	for i, tc := range toolCalls {
		result[i] = map[string]interface{}{
			"id":   tc.ID,
			"type": tc.Type,
			"function": map[string]interface{}{
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
			},
		}
	}

	return result
}

// transformTextResponse converts Mistral response to internal format
func (p *Provider) transformTextResponse(response *chatCompletionResponse) *types.TextResponse {
	if len(response.Choices) == 0 {
		return &types.TextResponse{
			ID:      response.ID,
			Model:   response.Model,
			Created: time.Unix(response.Created, 0),
		}
	}

	choice := response.Choices[0]

	return &types.TextResponse{
		ID:           response.ID,
		Model:        response.Model,
		Text:         choice.Message.Content,
		ToolCalls:    p.convertToolCalls(choice.Message.ToolCalls),
		FinishReason: p.mapFinishReason(choice.FinishReason),
		Usage:        p.convertUsage(response.Usage),
		Created:      time.Unix(response.Created, 0),
	}
}

// transformEmbeddingsResponse converts Mistral embeddings response
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
		result[i] = types.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
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
	case "tool_calls":
		return types.FinishReasonToolCalls
	case "content_filter":
		return types.FinishReasonContentFilter
	default:
		return types.FinishReasonStop
	}
}

// transformToolChoice converts tool choice to Mistral format
func (p *Provider) transformToolChoice(choice *types.ToolChoice) interface{} {
	if choice == nil {
		return "auto"
	}

	switch choice.Type {
	case types.ToolChoiceTypeNone:
		return "none"
	case types.ToolChoiceTypeAuto:
		return "auto"
	case types.ToolChoiceTypeAny:
		return "any"
	case types.ToolChoiceTypeSpecific:
		return map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": choice.ToolName,
			},
		}
	default:
		return "auto"
	}
}

func (p *Provider) schemaToTool(schema types.Schema, name string) (*types.Tool, error) {
	if name == "" {
		name = "structured_output"
	}

	// Convert Schema interface to map[string]interface{}
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var params map[string]interface{}
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
