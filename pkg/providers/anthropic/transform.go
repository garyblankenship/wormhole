package anthropic

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/prism-php/prism-go/pkg/types"
)

// buildMessagePayload builds the Anthropic messages API payload
func (p *Provider) buildMessagePayload(request *types.TextRequest) map[string]interface{} {
	payload := map[string]interface{}{
		"model":    request.Model,
		"messages": p.transformMessages(request.Messages),
	}

	// Add system prompt if present
	if request.SystemPrompt != "" {
		payload["system"] = request.SystemPrompt
	}

	// Handle max tokens - Anthropic requires this field
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		payload["max_tokens"] = *request.MaxTokens
	} else {
		payload["max_tokens"] = 4096 // Default
	}

	// Optional parameters
	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if len(request.Stop) > 0 {
		payload["stop_sequences"] = request.Stop
	}

	// Tools
	if len(request.Tools) > 0 {
		payload["tools"] = p.transformTools(request.Tools)
		if request.ToolChoice != nil {
			payload["tool_choice"] = request.ToolChoice
		}
	}

	// Provider options
	for k, v := range request.ProviderOptions {
		payload[k] = v
	}

	return payload
}

// transformMessages converts internal messages to Anthropic format
func (p *Provider) transformMessages(messages []types.Message) []map[string]interface{} {
	var result []map[string]interface{}

	for _, msg := range messages {
		// Skip system messages as they go in a separate field
		if msg.GetRole() == types.RoleSystem {
			continue
		}

		anthropicMsg := map[string]interface{}{
			"role": p.mapRole(msg.GetRole()),
		}

		// Build content array
		content := p.buildContent(msg)
		anthropicMsg["content"] = content

		result = append(result, anthropicMsg)
	}

	return result
}

// buildContent builds the content array for a message
func (p *Provider) buildContent(msg types.Message) []map[string]interface{} {
	var contentParts []map[string]interface{}

	content := msg.GetContent()

	switch c := content.(type) {
	case string:
		contentParts = append(contentParts, map[string]interface{}{
			"type": "text",
			"text": c,
		})
	case []types.MessagePart:
		for _, part := range c {
			if part.Type == "text" {
				contentParts = append(contentParts, map[string]interface{}{
					"type": "text",
					"text": part.Text,
				})
			} else if part.Type == "image" {
				contentParts = append(contentParts, map[string]interface{}{
					"type":   "image",
					"source": part.Data,
				})
			}
		}
	default:
		// Try to convert to string
		contentParts = append(contentParts, map[string]interface{}{
			"type": "text",
			"text": fmt.Sprintf("%v", content),
		})
	}

	// Handle tool messages
	if toolMsg, ok := msg.(*types.ToolMessage); ok {
		// Tool results are text content with tool_use_id
		if len(contentParts) > 0 {
			contentParts[0]["tool_use_id"] = toolMsg.ToolCallID
		}
	}

	// Handle assistant messages with tool calls
	if assistantMsg, ok := msg.(*types.AssistantMessage); ok && len(assistantMsg.ToolCalls) > 0 {
		for _, toolCall := range assistantMsg.ToolCalls {
			var input map[string]interface{}
			json.Unmarshal([]byte(toolCall.Function.Arguments), &input)
			contentParts = append(contentParts, map[string]interface{}{
				"type":  "tool_use",
				"id":    toolCall.ID,
				"name":  toolCall.Function.Name,
				"input": input,
			})
		}
	}

	return contentParts
}

// mapRole maps internal roles to Anthropic roles
func (p *Provider) mapRole(role types.Role) string {
	switch role {
	case types.RoleUser:
		return "user"
	case types.RoleAssistant:
		return "assistant"
	case types.RoleTool:
		return "user" // Anthropic uses 'user' role for tool results
	default:
		return string(role)
	}
}

// transformTools converts internal tools to Anthropic format
func (p *Provider) transformTools(tools []types.Tool) []map[string]interface{} {
	result := make([]map[string]interface{}, len(tools))

	for i, tool := range tools {
		parameters, _ := json.Marshal(tool.Function.Parameters)
		result[i] = map[string]interface{}{
			"name":         tool.Function.Name,
			"description":  tool.Function.Description,
			"input_schema": json.RawMessage(parameters),
		}
	}

	return result
}

// transformTextResponse converts Anthropic response to internal format
func (p *Provider) transformTextResponse(response *messageResponse) *types.TextResponse {
	text := ""
	var toolCalls []types.ToolCall

	// Extract content from response
	for _, content := range response.Content {
		if content.Type == "text" {
			text += content.Text
		} else if content.Type == "tool_use" {
			args, _ := json.Marshal(content.Input)
			toolCalls = append(toolCalls, types.ToolCall{
				ID:   content.ID,
				Type: "function",
				Function: &types.ToolCallFunction{
					Name:      content.Name,
					Arguments: string(args),
				},
			})
		}
	}

	return &types.TextResponse{
		ID:           response.ID,
		Model:        response.Model,
		Text:         text,
		ToolCalls:    toolCalls,
		FinishReason: p.mapStopReason(response.StopReason),
		Usage:        p.convertUsage(response.Usage),
		Created:      time.Now(),
	}
}

// parseStreamChunk parses a streaming chunk
func (p *Provider) parseStreamChunk(data []byte) (*types.StreamChunk, error) {
	// First, determine the event type
	var baseEvent streamEvent
	if err := json.Unmarshal(data, &baseEvent); err != nil {
		return nil, err
	}

	chunk := &types.StreamChunk{}

	switch baseEvent.Type {
	case "message_start":
		var event messageStartEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		chunk.ID = event.Message.ID
		chunk.Model = event.Message.Model

	case "content_block_delta":
		var event contentBlockDeltaEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		if event.Delta.Type == "text_delta" {
			chunk.Delta = &types.ChunkDelta{
				Content: event.Delta.Text,
			}
		}

	case "message_delta":
		var event messageDeltaEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		if event.Delta.StopReason != "" {
			reason := p.mapStopReason(event.Delta.StopReason)
			chunk.FinishReason = &reason
		}
		if event.Delta.Usage.InputTokens > 0 || event.Delta.Usage.OutputTokens > 0 {
			chunk.Usage = p.convertUsage(event.Delta.Usage)
		}

	case "message_stop":
		// End of stream
		return nil, nil
	}

	return chunk, nil
}

// Helper functions

func (p *Provider) convertUsage(u messageUsage) *types.Usage {
	return &types.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.InputTokens + u.OutputTokens,
	}
}

func (p *Provider) mapStopReason(reason string) types.FinishReason {
	switch reason {
	case "end_turn":
		return types.FinishReasonStop
	case "max_tokens":
		return types.FinishReasonLength
	case "tool_use":
		return types.FinishReasonToolCalls
	default:
		return types.FinishReasonStop
	}
}

func (p *Provider) schemaToTool(schema json.RawMessage, name string) (*types.Tool, error) {
	if name == "" {
		name = "structured_output"
	}

	// Convert json.RawMessage to map[string]interface{}
	var params map[string]interface{}
	if err := json.Unmarshal(schema, &params); err != nil {
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
