package ollama

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/prism-php/prism-go/pkg/types"
)

// buildChatPayload builds the Ollama chat completion payload
func (p *Provider) buildChatPayload(request *types.TextRequest) *chatRequest {
	payload := &chatRequest{
		Model:    request.Model,
		Messages: p.transformMessages(request.Messages, request.SystemPrompt),
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
		if presencePenalty, ok := request.ProviderOptions["presence_penalty"].(float32); ok {
			opts.PresencePenalty = &presencePenalty
			hasOptions = true
		}
		if frequencyPenalty, ok := request.ProviderOptions["frequency_penalty"].(float32); ok {
			opts.FrequencyPenalty = &frequencyPenalty
			hasOptions = true
		}
	}

	if !hasOptions {
		return nil
	}
	return opts
}

// transformMessages converts internal messages to Ollama format
func (p *Provider) transformMessages(messages []types.Message, systemPrompt string) []message {
	var result []message

	// Add system prompt as first message if provided
	if systemPrompt != "" {
		result = append(result, message{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	for _, msg := range messages {
		ollamaMsg := message{
			Role: p.mapRole(msg.GetRole()),
		}

		// Handle content based on type
		content := msg.GetContent()
		switch c := content.(type) {
		case string:
			ollamaMsg.Content = c
		case []types.MessagePart:
			// Handle multimodal content
			textParts := []string{}
			images := []string{}

			for _, part := range c {
				if part.Type == "text" {
					textParts = append(textParts, part.Text)
				} else if part.Type == "image" {
					// Extract base64 data from data URL if needed
					imageData, ok := part.Data.(string)
					if !ok {
						imageData = fmt.Sprintf("%v", part.Data)
					}
					if strings.HasPrefix(imageData, "data:image/") {
						// Extract base64 part from data URL
						if idx := strings.Index(imageData, ","); idx != -1 {
							imageData = imageData[idx+1:]
						}
					}
					images = append(images, imageData)
				}
			}

			// Combine text parts
			if len(textParts) > 0 {
				ollamaMsg.Content = strings.Join(textParts, "\n")
			}
			if len(images) > 0 {
				ollamaMsg.Images = images
			}
		default:
			// Try to convert to string
			ollamaMsg.Content = fmt.Sprintf("%v", content)
		}

		result = append(result, ollamaMsg)
	}

	return result
}

// mapRole maps internal role to Ollama role
func (p *Provider) mapRole(role types.Role) string {
	switch role {
	case types.RoleSystem:
		return "system"
	case types.RoleUser:
		return "user"
	case types.RoleAssistant:
		return "assistant"
	case types.RoleTool:
		return "tool" // Ollama may not support this, treat as user
	default:
		return "user"
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
		FinishReason: p.mapFinishReason(response.Done),
		Usage:        p.convertUsage(response),
		Created:      response.CreatedAt,
	}
}

// transformEmbeddingsResponse converts Ollama embeddings response
func (p *Provider) transformEmbeddingsResponse(response *embeddingsResponse, model string) *types.EmbeddingsResponse {
	embeddings := []types.Embedding{
		{
			Index:     0,
			Embedding: response.Embedding,
		},
	}

	return &types.EmbeddingsResponse{
		Model:      model,
		Embeddings: embeddings,
		Usage:      nil, // Ollama doesn't provide usage info for embeddings
		Created:    time.Now(),
	}
}

// parseStreamChunk parses a streaming chunk from Ollama
func (p *Provider) parseStreamChunk(data []byte) (*types.TextChunk, error) {
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
		reason := p.mapFinishReason(response.Done)
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

// mapFinishReason maps Ollama's done status to finish reason
func (p *Provider) mapFinishReason(done bool) types.FinishReason {
	if done {
		return types.FinishReasonStop
	}
	return types.FinishReasonStop // Default to stop
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

// buildEmbeddingsPayload builds the Ollama embeddings payload
func (p *Provider) buildEmbeddingsPayload(request *types.EmbeddingsRequest) *embeddingsRequest {
	// Ollama embeddings API takes a single string prompt
	// If multiple inputs, we'll process them individually
	var prompt string
	if len(request.Input) > 0 {
		prompt = request.Input[0] // Use first input for now
	}

	return &embeddingsRequest{
		Model:  request.Model,
		Prompt: prompt,
	}
}
