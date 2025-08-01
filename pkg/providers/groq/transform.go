package groq

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/prism-php/prism-go/internal/utils"
	"github.com/prism-php/prism-go/pkg/types"
)

// transformMessages converts types.Message to Groq/OpenAI format
func (g *Groq) transformMessages(messages []types.Message) []map[string]interface{} {
	var transformed []map[string]interface{}

	for _, msg := range messages {
		message := g.transformMessage(msg)
		if message != nil {
			transformed = append(transformed, message)
		}
	}

	return transformed
}

// transformMessage converts a single message to Groq format
func (g *Groq) transformMessage(msg types.Message) map[string]interface{} {
	switch m := msg.(type) {
	case *types.UserMessage:
		content := g.buildMessageContent(m.Content, m.Media)
		return map[string]interface{}{
			"role":    "user",
			"content": content,
		}

	case *types.AssistantMessage:
		message := map[string]interface{}{
			"role":    "assistant",
			"content": m.Content,
		}

		// Add tool calls if present
		if len(m.ToolCalls) > 0 {
			var toolCalls []map[string]interface{}
			for _, tc := range m.ToolCalls {
				args, _ := json.Marshal(tc.Arguments)
				toolCalls = append(toolCalls, map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": string(args),
					},
				})
			}
			message["tool_calls"] = toolCalls
		}

		return message

	case *types.ToolResultMessage:
		return map[string]interface{}{
			"role":         "tool",
			"content":      m.Content,
			"tool_call_id": m.ToolCallID,
		}

	case *types.SystemMessage:
		return map[string]interface{}{
			"role":    "system",
			"content": m.Content,
		}

	default:
		return nil
	}
}

// buildMessageContent builds content with media support
func (g *Groq) buildMessageContent(text string, media []types.Media) interface{} {
	if len(media) == 0 {
		return text
	}

	// Groq supports multimodal input similar to OpenAI
	var content []map[string]interface{}

	// Add text part if present
	if text != "" {
		content = append(content, map[string]interface{}{
			"type": "text",
			"text": text,
		})
	}

	// Add media parts
	for _, m := range media {
		switch mediaItem := m.(type) {
		case *types.ImageMedia:
			imageContent := map[string]interface{}{
				"type": "image_url",
			}

			if mediaItem.URL != "" {
				imageContent["image_url"] = map[string]interface{}{
					"url": mediaItem.URL,
				}
			} else if len(mediaItem.Data) > 0 {
				// Convert to data URL
				imageContent["image_url"] = map[string]interface{}{
					"url": fmt.Sprintf("data:%s;base64,%s",
						mediaItem.MimeType,
						mediaItem.Base64Data),
				}
			}

			content = append(content, imageContent)
		}
		// Groq doesn't support other media types yet
	}

	return content
}

// transformTools converts tools to Groq/OpenAI format
func (g *Groq) transformTools(tools []types.Tool) []map[string]interface{} {
	var transformed []map[string]interface{}

	for _, tool := range tools {
		transformed = append(transformed, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.InputSchema,
			},
		})
	}

	return transformed
}

// transformToolChoice converts tool choice to Groq/OpenAI format
func (g *Groq) transformToolChoice(choice *types.ToolChoice) interface{} {
	if choice == nil {
		return "auto"
	}

	switch choice.Type {
	case types.ToolChoiceTypeNone:
		return "none"
	case types.ToolChoiceTypeAuto:
		return "auto"
	case types.ToolChoiceTypeAny:
		return "required"
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

// transformTextResponse converts Groq response to types.TextResponse
func (g *Groq) transformTextResponse(response *groqTextResponse) (*types.TextResponse, error) {
	if response.Error != nil {
		return nil, errors.New(response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return nil, errors.New("no choices in response")
	}

	choice := response.Choices[0]

	// Convert tool calls
	var toolCalls []types.ToolCall
	for _, tc := range choice.Message.ToolCalls {
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			// If unmarshaling fails, treat as string
			args = map[string]interface{}{
				"_raw": tc.Function.Arguments,
			}
		}

		toolCalls = append(toolCalls, types.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	finishReason := types.FinishReasonStop
	if mapped, ok := finishReasonMap[choice.FinishReason]; ok {
		finishReason = mapped
	}

	result := &types.TextResponse{
		Text:         choice.Message.Content,
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
	}

	// Add usage if available
	if response.Usage != nil {
		result.Usage = &types.Usage{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		}
	}

	// Add metadata
	result.Metadata = map[string]interface{}{
		"provider": "groq",
		"model":    response.Model,
	}

	return result, nil
}

// transformStructuredResponse converts Groq response to types.StructuredResponse
func (g *Groq) transformStructuredResponse(response *groqTextResponse, schema types.Schema) (*types.StructuredResponse, error) {
	if response.Error != nil {
		return nil, errors.New(response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return nil, errors.New("no choices in response")
	}

	choice := response.Choices[0]

	// Parse JSON response
	var data interface{}
	if err := json.Unmarshal([]byte(choice.Message.Content), &data); err != nil {
		return nil, fmt.Errorf("failed to parse structured response: %w", err)
	}

	// Validate against schema if it implements SchemaInterface
	if schemaIface, ok := schema.(types.SchemaInterface); ok {
		if err := schemaIface.Validate(data); err != nil {
			return nil, fmt.Errorf("response validation failed: %w", err)
		}
	}

	result := &types.StructuredResponse{
		Data: data,
		Raw:  choice.Message.Content,
	}

	// Add usage if available
	if response.Usage != nil {
		result.Usage = &types.Usage{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		}
	}

	// Add metadata
	result.Metadata = map[string]interface{}{
		"provider": "groq",
		"model":    response.Model,
	}

	return result, nil
}

// handleStream processes streaming responses
func (g *Groq) handleStream(stream io.ReadCloser) <-chan types.TextChunk {
	ch := make(chan types.TextChunk)

	go func() {
		defer close(ch)
		defer stream.Close()

		scanner := utils.NewSSEScanner(stream)
		for scanner.Scan() {
			event := scanner.Event()
			if event.Data == "" {
				continue
			}

			// Skip [DONE] message
			if strings.TrimSpace(event.Data) == "[DONE]" {
				return
			}

			var response groqStreamResponse
			if err := json.Unmarshal([]byte(event.Data), &response); err != nil {
				ch <- types.TextChunk{Error: err}
				return
			}

			if response.Error != nil {
				ch <- types.TextChunk{Error: errors.New(response.Error.Message)}
				return
			}

			if len(response.Choices) > 0 {
				choice := response.Choices[0]

				// Send content if present
				if choice.Delta.Content != "" {
					ch <- types.TextChunk{
						Text:  choice.Delta.Content,
						Model: response.Model,
					}
				}

				// Send tool calls if present
				for _, tc := range choice.Delta.ToolCalls {
					var args map[string]interface{}
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						args = map[string]interface{}{
							"_raw": tc.Function.Arguments,
						}
					}

					ch <- types.TextChunk{
						ToolCall: &types.ToolCall{
							ID:        tc.ID,
							Name:      tc.Function.Name,
							Arguments: args,
						},
						Model: response.Model,
					}
				}

				// Send finish reason if present
				if choice.FinishReason != "" {
					finishReason := types.FinishReasonStop
					if mapped, ok := finishReasonMap[choice.FinishReason]; ok {
						finishReason = mapped
					}
					ch <- types.TextChunk{
						FinishReason: &finishReason,
						Model:        response.Model,
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- types.TextChunk{Error: err}
		}
	}()

	return ch
}
