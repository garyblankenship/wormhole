package gemini

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// transformMessages converts types.Message to Gemini format
func (g *Gemini) transformMessages(messages []types.Message) ([]map[string]interface{}, error) {
	var contents []map[string]interface{}

	for _, msg := range messages {
		content := map[string]interface{}{
			"role": g.mapRole(string(msg.GetRole())),
		}

		parts, err := g.transformMessageToParts(msg)
		if err != nil {
			return nil, err
		}

		content["parts"] = parts
		contents = append(contents, content)
	}

	return contents, nil
}

// mapRole maps message roles to Gemini roles
func (g *Gemini) mapRole(role string) string {
	switch role {
	case "system":
		return "model"
	case "assistant":
		return "model"
	case "tool":
		return "function"
	default:
		return "user"
	}
}

// transformMessageToParts converts a message to Gemini parts
func (g *Gemini) transformMessageToParts(msg types.Message) ([]map[string]interface{}, error) {
	var parts []map[string]interface{}

	switch m := msg.(type) {
	case *types.UserMessage:
		if m.Content != "" {
			parts = append(parts, map[string]interface{}{"text": m.Content})
		}

		// Handle media
		for _, media := range m.Media {
			part, err := g.transformMedia(media)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}

	case *types.AssistantMessage:
		if m.Content != "" {
			parts = append(parts, map[string]interface{}{"text": m.Content})
		}

		// Handle tool calls
		for _, toolCall := range m.ToolCalls {
			parts = append(parts, map[string]interface{}{
				"functionCall": map[string]interface{}{
					"name": toolCall.Name,
					"args": toolCall.Arguments,
				},
			})
		}

	case *types.ToolResultMessage:
		parts = append(parts, map[string]interface{}{
			"functionResponse": map[string]interface{}{
				"name": m.ToolCallID,
				"response": map[string]interface{}{
					"result": m.Content,
				},
			},
		})

	case *types.SystemMessage:
		parts = append(parts, map[string]interface{}{"text": m.Content})

	default:
		return nil, fmt.Errorf("unsupported message type: %T", msg)
	}

	return parts, nil
}

// transformMedia converts media to Gemini format
func (g *Gemini) transformMedia(media types.Media) (map[string]interface{}, error) {
	switch m := media.(type) {
	case *types.ImageMedia:
		if m.URL != "" {
			// Gemini requires base64 encoded images
			return nil, errors.New("Gemini requires base64 encoded images, URLs are not supported")
		}

		return map[string]interface{}{
			"inlineData": map[string]interface{}{
				"mimeType": m.MimeType,
				"data":     base64.StdEncoding.EncodeToString(m.Data),
			},
		}, nil

	case *types.DocumentMedia:
		return map[string]interface{}{
			"inlineData": map[string]interface{}{
				"mimeType": m.MimeType,
				"data":     base64.StdEncoding.EncodeToString(m.Data),
			},
		}, nil

	default:
		return nil, fmt.Errorf("unsupported media type: %T", media)
	}
}

// transformTools converts tools to Gemini format
func (g *Gemini) transformTools(tools []types.Tool) ([]map[string]interface{}, error) {
	var geminiTools []map[string]interface{}

	var functions []map[string]interface{}
	for _, tool := range tools {
		schema := g.transformToolSchema(tool.InputSchema)
		functions = append(functions, map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  schema,
		})
	}

	if len(functions) > 0 {
		geminiTools = append(geminiTools, map[string]interface{}{
			"functionDeclarations": functions,
		})
	}

	return geminiTools, nil
}

// transformToolSchema converts tool schema to Gemini format
func (g *Gemini) transformToolSchema(schema map[string]interface{}) map[string]interface{} {
	// Gemini expects JSON Schema format
	if _, ok := schema["type"]; !ok {
		schema["type"] = "object"
	}
	return schema
}

// transformToolChoice converts tool choice to Gemini format
func (g *Gemini) transformToolChoice(choice *types.ToolChoice) map[string]interface{} {
	if choice == nil {
		return nil
	}

	switch choice.Type {
	case types.ToolChoiceTypeAuto:
		return map[string]interface{}{
			"functionCallingConfig": map[string]interface{}{
				"mode": "AUTO",
			},
		}
	case types.ToolChoiceTypeNone:
		return map[string]interface{}{
			"functionCallingConfig": map[string]interface{}{
				"mode": "NONE",
			},
		}
	case types.ToolChoiceTypeAny:
		return map[string]interface{}{
			"functionCallingConfig": map[string]interface{}{
				"mode": "ANY",
			},
		}
	case types.ToolChoiceTypeSpecific:
		return map[string]interface{}{
			"functionCallingConfig": map[string]interface{}{
				"mode":                 "ANY",
				"allowedFunctionNames": []string{choice.ToolName},
			},
		}
	default:
		return nil
	}
}

// transformSchema converts a types.Schema to Gemini schema format
func (g *Gemini) transformSchema(schema types.Schema) map[string]interface{} {
	return g.schemaToMap(schema)
}

// schemaToMap recursively converts schema to map
func (g *Gemini) schemaToMap(schema types.Schema) map[string]interface{} {
	// Handle raw JSON bytes
	if bytes, ok := schema.([]byte); ok {
		var result map[string]interface{}
		if err := json.Unmarshal(bytes, &result); err == nil {
			return result
		}
	}

	// Handle schema interface
	if schemaIface, ok := schema.(types.SchemaInterface); ok {
		result := map[string]interface{}{
			"type": schemaIface.GetType(),
		}

		if desc := schemaIface.GetDescription(); desc != "" {
			result["description"] = desc
		}
		return result
	}

	// Default empty result
	result := map[string]interface{}{}

	switch s := schema.(type) {
	case *types.ObjectSchema:
		properties := make(map[string]interface{})
		for name, prop := range s.Properties {
			properties[name] = g.schemaToMap(prop)
		}
		result["properties"] = properties
		if len(s.Required) > 0 {
			result["required"] = s.Required
		}

	case *types.ArraySchema:
		result["items"] = g.schemaToMap(s.Items)

	case *types.EnumSchema:
		result["enum"] = s.Enum

	case *types.NumberSchema:
		if s.Minimum != nil {
			result["minimum"] = *s.Minimum
		}
		if s.Maximum != nil {
			result["maximum"] = *s.Maximum
		}

	case *types.StringSchema:
		if s.MinLength != nil {
			result["minLength"] = *s.MinLength
		}
		if s.MaxLength != nil {
			result["maxLength"] = *s.MaxLength
		}
		if s.Pattern != "" {
			result["pattern"] = s.Pattern
		}
	}

	return result
}

// transformTextResponse converts Gemini response to types.TextResponse
func (g *Gemini) transformTextResponse(response *geminiTextResponse) (*types.TextResponse, error) {
	if response.Error != nil {
		return nil, errors.New(response.Error.Message)
	}

	if len(response.Candidates) == 0 {
		return nil, errors.New("no candidates in response")
	}

	candidate := response.Candidates[0]

	// Extract text and tool calls
	var text string
	var toolCalls []types.ToolCall

	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			text += part.Text
		}
		if part.FunctionCall != nil {
			toolCalls = append(toolCalls, types.ToolCall{
				ID:        part.FunctionCall.Name, // Gemini doesn't provide IDs
				Name:      part.FunctionCall.Name,
				Arguments: part.FunctionCall.Args,
			})
		}
	}

	finishReason := types.FinishReasonStop
	if mappedReason, ok := finishReasonMap[candidate.FinishReason]; ok {
		finishReason = mappedReason
	}

	result := &types.TextResponse{
		Text:         text,
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
	}

	// Add usage if available
	if response.UsageMetadata != nil {
		result.Usage = &types.Usage{
			PromptTokens:     response.UsageMetadata.PromptTokenCount,
			CompletionTokens: response.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      response.UsageMetadata.TotalTokenCount,
		}
	}

	// Add metadata
	result.Metadata = map[string]interface{}{
		"provider": "gemini",
	}

	if candidate.GroundingMetadata != nil {
		result.Metadata["groundingMetadata"] = candidate.GroundingMetadata
	}

	return result, nil
}

// transformStructuredResponse converts Gemini response to types.StructuredResponse
func (g *Gemini) transformStructuredResponse(response *geminiTextResponse, schema types.Schema) (*types.StructuredResponse, error) {
	if response.Error != nil {
		return nil, errors.New(response.Error.Message)
	}

	if len(response.Candidates) == 0 {
		return nil, errors.New("no candidates in response")
	}

	candidate := response.Candidates[0]

	// Extract text (should be JSON)
	var text string
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			text += part.Text
		}
	}

	// Parse JSON
	var data interface{}
	if err := json.Unmarshal([]byte(text), &data); err != nil {
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
		Raw:  text,
	}

	// Add usage if available
	if response.UsageMetadata != nil {
		result.Usage = &types.Usage{
			PromptTokens:     response.UsageMetadata.PromptTokenCount,
			CompletionTokens: response.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      response.UsageMetadata.TotalTokenCount,
		}
	}

	// Add metadata
	result.Metadata = map[string]interface{}{
		"provider": "gemini",
	}

	return result, nil
}

// transformEmbeddingsResponse converts Gemini response to types.EmbeddingsResponse
func (g *Gemini) transformEmbeddingsResponse(response *geminiEmbeddingsResponse) (*types.EmbeddingsResponse, error) {
	var embeddings []types.Embedding

	for i, emb := range response.Embeddings {
		embeddings = append(embeddings, types.Embedding{
			Index:     i,
			Embedding: emb.Values,
		})
	}

	return &types.EmbeddingsResponse{
		Embeddings: embeddings,
		Metadata: map[string]interface{}{
			"provider": "gemini",
		},
	}, nil
}

// handleStream processes streaming responses
func (g *Gemini) handleStream(stream io.ReadCloser) <-chan types.TextChunk {
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

			var response geminiTextResponse
			if err := json.Unmarshal([]byte(event.Data), &response); err != nil {
				ch <- types.TextChunk{Error: err}
				return
			}

			if response.Error != nil {
				ch <- types.TextChunk{Error: errors.New(response.Error.Message)}
				return
			}

			if len(response.Candidates) > 0 {
				candidate := response.Candidates[0]

				for _, part := range candidate.Content.Parts {
					if part.Text != "" {
						ch <- types.TextChunk{
							Text:  part.Text,
							Model: "gemini",
						}
					}

					if part.FunctionCall != nil {
						ch <- types.TextChunk{
							ToolCall: &types.ToolCall{
								ID:        part.FunctionCall.Name,
								Name:      part.FunctionCall.Name,
								Arguments: part.FunctionCall.Args,
							},
							Model: "gemini",
						}
					}
				}

				// Send finish reason if present
				if candidate.FinishReason != "" {
					finishReason := types.FinishReasonStop
					if mapped, ok := finishReasonMap[candidate.FinishReason]; ok {
						finishReason = mapped
					}
					ch <- types.TextChunk{
						FinishReason: &finishReason,
						Model:        "gemini",
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
