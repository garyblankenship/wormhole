package openai

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/garyblankenship/wormhole/v2/providers"
	"github.com/garyblankenship/wormhole/v2/types"
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
	if request.FrequencyPenalty != nil {
		payload["frequency_penalty"] = *request.FrequencyPenalty
	}
	if request.PresencePenalty != nil {
		payload["presence_penalty"] = *request.PresencePenalty
	}
	if request.Seed != nil {
		payload["seed"] = *request.Seed
	}
	if request.ParallelToolCalls != nil {
		payload["parallel_tool_calls"] = *request.ParallelToolCalls
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
