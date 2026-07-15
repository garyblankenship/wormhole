package ollama

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/v2/providers"
	"github.com/garyblankenship/wormhole/v2/types"
)

// Role mapping constants
const (
	roleSystem    = "system"
	roleUser      = "user"
	roleAssistant = "assistant"
	roleTool      = "tool"
)

// buildChatPayload builds the Ollama chat completion payload
func (p *Provider) buildChatPayload(request *types.TextRequest) *chatRequest {
	prepared, _, err := providers.PrepareMessages(request.Messages)
	if err != nil {
		prepared = request.Messages // fall through; provider will surface the issue
	}
	payload := &chatRequest{
		Model:    request.Model,
		Messages: p.transformMessages(prepared, request.SystemPrompt),
		Options:  p.buildOptions(request),
	}

	// Set JSON format for structured output
	if request.ResponseFormat != nil {
		if rf, ok := request.ResponseFormat.(map[string]string); ok {
			if rf["type"] == "json_object" {
				payload.Format = "json"
			}
		}
	}

	return payload
}

// buildOptions builds Ollama options from the request
func (p *Provider) buildOptions(request *types.TextRequest) *options {
	opts := &options{}
	hasOptions := false

	// Basic parameters
	if request.Temperature != nil {
		opts.Temperature = request.Temperature
		hasOptions = true
	}
	if request.TopP != nil {
		opts.TopP = request.TopP
		hasOptions = true
	}
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		opts.NumPredict = request.MaxTokens
		hasOptions = true
	}
	if len(request.Stop) > 0 {
		opts.Stop = request.Stop
		hasOptions = true
	}
	if request.Seed != nil {
		opts.Seed = request.Seed
		hasOptions = true
	}
	if request.PresencePenalty != nil {
		opts.PresencePenalty = request.PresencePenalty
		hasOptions = true
	}
	if request.FrequencyPenalty != nil {
		opts.FrequencyPenalty = request.FrequencyPenalty
		hasOptions = true
	}

	// Provider-specific options
	if request.ProviderOptions != nil {
		if topK, ok := request.ProviderOptions["top_k"].(int); ok {
			opts.TopK = &topK
			hasOptions = true
		}
		if repeatPenalty, ok := request.ProviderOptions["repeat_penalty"].(float32); ok {
			opts.RepeatPenalty = &repeatPenalty
			hasOptions = true
		}
		if request.PresencePenalty == nil {
			if presencePenalty, ok := request.ProviderOptions["presence_penalty"].(float32); ok {
				opts.PresencePenalty = &presencePenalty
				hasOptions = true
			}
		}
		if request.FrequencyPenalty == nil {
			if frequencyPenalty, ok := request.ProviderOptions["frequency_penalty"].(float32); ok {
				opts.FrequencyPenalty = &frequencyPenalty
				hasOptions = true
			}
		}
	}

	if !hasOptions {
		return nil
	}
	return opts
}

// extractImageData extracts base64 image data from data URLs or raw strings
func extractImageData(data any) string {
	imageData, ok := data.(string)
	if !ok {
		return fmt.Sprintf("%v", data)
	}
	// Extract base64 part from data URL if present
	if strings.HasPrefix(imageData, "data:image/") {
		if idx := strings.Index(imageData, ","); idx != -1 {
			return imageData[idx+1:]
		}
	}
	return imageData
}

// convertMultimodalParts processes message parts into text and images
func convertMultimodalParts(parts []types.MessagePart) (string, []string) {
	textParts := make([]string, 0, len(parts))
	images := make([]string, 0)

	for _, part := range parts {
		switch part.Type {
		case "text":
			textParts = append(textParts, part.Text)
		case "image":
			images = append(images, extractImageData(part.Data))
		}
	}

	return strings.Join(textParts, "\n"), images
}

// extractMediaImages pulls base64-encoded image data from a UserMessage's Media
// slice into the Ollama `images` wire field. Ollama expects raw base64 (no data:
// URL prefix). URL-only images (image.URL set) are skipped because Ollama's chat
// API takes inline base64, not remote URLs.
func extractMediaImages(media []types.Media) []string {
	images := make([]string, 0, len(media))
	for _, m := range media {
		image, ok := m.(*types.ImageMedia)
		if !ok {
			continue
		}
		data := image.Base64Data
		if data == "" && len(image.Data) > 0 {
			data = base64.StdEncoding.EncodeToString(image.Data)
		}
		if data == "" {
			continue
		}
		images = append(images, data)
	}
	if len(images) == 0 {
		return nil
	}
	return images
}

// transformMessages converts internal messages to Ollama format
func (p *Provider) transformMessages(messages []types.Message, systemPrompt string) []message {
	capacity := len(messages)
	if systemPrompt != "" {
		capacity++
	}
	result := make([]message, 0, capacity)

	if systemPrompt != "" {
		result = append(result, message{
			Role:    roleSystem,
			Content: systemPrompt,
		})
	}

	for _, msg := range messages {
		ollamaMsg := message{Role: p.mapRole(msg.GetRole())}

		switch c := msg.GetContent().(type) {
		case string:
			ollamaMsg.Content = c
		case []types.MessagePart:
			text, images := convertMultimodalParts(c)
			if text != "" {
				ollamaMsg.Content = text
			}
			if len(images) > 0 {
				ollamaMsg.Images = images
			}
		default:
			ollamaMsg.Content = fmt.Sprintf("%v", c)
		}

		// UserMessage.Media carries images set by the proxy/ImageMedia path,
		// which the content switch above does not see. Pull them in if the
		// MessagePart path did not already populate Images.
		if len(ollamaMsg.Images) == 0 {
			if userMsg, ok := msg.(*types.UserMessage); ok && len(userMsg.Media) > 0 {
				if imgs := extractMediaImages(userMsg.Media); len(imgs) > 0 {
					ollamaMsg.Images = imgs
				}
			}
		}

		result = append(result, ollamaMsg)
	}

	return result
}

// mapRole maps internal role to Ollama role
func (p *Provider) mapRole(role types.Role) string {
	switch role {
	case types.RoleSystem:
		return roleSystem
	case types.RoleUser:
		return roleUser
	case types.RoleAssistant:
		return roleAssistant
	case types.RoleTool:
		return roleTool // Ollama may not support this, treat as user
	default:
		return roleUser
	}
}

// transformTextResponse converts Ollama response to internal format
func (p *Provider) transformTextResponse(response *chatResponse) *types.TextResponse {
	// Generate a simple ID since Ollama doesn't provide one
	id := fmt.Sprintf("ollama_%d", time.Now().UnixNano())

	// Extract content as string
	var content string
	if str, ok := response.Message.Content.(string); ok {
		content = str
	} else {
		content = fmt.Sprintf("%v", response.Message.Content)
	}

	return &types.TextResponse{
		ID:           id,
		Model:        response.Model,
		Text:         content,
		FinishReason: p.mapFinishReason(response.DoneReason),
		Usage:        p.convertUsage(response),
		Created:      response.CreatedAt,
	}
}

// parseStreamChunk parses a streaming chunk from Ollama
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

	// Generate a simple ID since Ollama doesn't provide one
	id := fmt.Sprintf("ollama_%d", time.Now().UnixNano())

	// Extract content as string
	var content string
	if str, ok := response.Message.Content.(string); ok {
		content = str
	} else {
		content = fmt.Sprintf("%v", response.Message.Content)
	}

	chunk := &types.StreamChunk{
		ID:    id,
		Model: response.Model,
		Delta: &types.ChunkDelta{
			Content: content,
		},
	}

	if response.Done {
		reason := p.mapFinishReason(response.DoneReason)
		chunk.FinishReason = &reason
	}

	if response.Done {
		chunk.Usage = p.convertUsage(&chatResponse{
			Model:              response.Model,
			CreatedAt:          response.CreatedAt,
			TotalDuration:      response.TotalDuration,
			LoadDuration:       response.LoadDuration,
			PromptEvalCount:    response.PromptEvalCount,
			PromptEvalDuration: response.PromptEvalDuration,
			EvalCount:          response.EvalCount,
			EvalDuration:       response.EvalDuration,
		})
	}

	return chunk, nil
}

// Helper functions

// mapFinishReason maps Ollama's done_reason to finish reason.
// Ollama returns done_reason values: "stop", "length", "load", "unload".
func (p *Provider) mapFinishReason(doneReason string) types.FinishReason {
	switch doneReason {
	case "stop":
		return types.FinishReasonStop
	case "length":
		return types.FinishReasonLength
	case "load", "unload":
		// Model load/unload — not a normal generation stop.
		return types.FinishReasonOther
	default:
		return types.FinishReasonOther
	}
}

// convertUsage converts Ollama response to usage info
func (p *Provider) convertUsage(response *chatResponse) *types.Usage {
	if response == nil {
		return nil
	}

	// Calculate token usage from Ollama's eval counts
	// Ollama provides prompt_eval_count and eval_count
	promptTokens := response.PromptEvalCount
	completionTokens := response.EvalCount
	totalTokens := promptTokens + completionTokens

	return &types.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}
