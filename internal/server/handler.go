package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	wormhole "github.com/garyblankenship/wormhole/v2"
	"github.com/garyblankenship/wormhole/v2/types"
)

const maxProxyRequestBodyBytes = 20 << 20

func (p *proxy) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func parseChatMessages(input []ChatCompletionRequestMessage) ([]types.Message, error) {
	messages := make([]types.Message, 0, len(input))
	for _, message := range input {
		if message.Role != "user" && len(message.Content.Media) > 0 {
			return nil, fmt.Errorf("image content parts are only supported on user messages")
		}
		switch message.Role {
		case "system", "developer":
			messages = append(messages, types.NewSystemMessage(message.Content.Text))
		case "user":
			messages = append(messages, &types.UserMessage{Content: message.Content.Text, Media: message.Content.Media})
		case "assistant":
			assistant := types.NewAssistantMessage(message.Content.Text)
			if len(message.ToolCalls) > 0 {
				toolCalls, err := toWormholeToolCalls(message.ToolCalls)
				if err != nil {
					return nil, err
				}
				assistant.ToolCalls = toolCalls
			}
			messages = append(messages, assistant)
		case "tool", "function":
			messages = append(messages, types.NewToolResultMessage(message.ToolCallID, message.Content.Text))
		default:
			return nil, fmt.Errorf("unsupported message role %q", message.Role)
		}
	}
	return messages, nil
}

func parseChatToolConfig(input []ChatTool, rawChoice json.RawMessage) ([]types.Tool, *types.ToolChoice, error) {
	tools, err := toWormholeTools(input)
	if err != nil {
		return nil, nil, err
	}
	choice, err := parseToolChoice(rawChoice)
	if err != nil {
		return nil, nil, err
	}
	if choice == nil {
		return tools, nil, nil
	}
	if choice.Type == types.ToolChoiceTypeAny && len(tools) == 0 {
		return nil, nil, fmt.Errorf("tool_choice %q requires at least one declared tool", "required")
	}
	if choice.Type == types.ToolChoiceTypeSpecific {
		for _, tool := range tools {
			if tool.Name == choice.ToolName {
				return tools, choice, nil
			}
		}
		return nil, nil, fmt.Errorf("selected tool %q is not declared", choice.ToolName)
	}
	return tools, choice, nil
}

func (p *proxy) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req ChatCompletionRequest
	if err := decodeRequestBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json",
			"Failed to parse request body: "+err.Error(), "invalid_request_error")
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model_required",
			"model is required", "invalid_request_error")
		return
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages_required",
			"messages is required", "invalid_request_error")
		return
	}

	configuredProviders := p.wh.ConfiguredProviders()
	effDefaultProvider := effectiveDefaultProvider(p.defaultProvider, configuredProviders)
	provider, model := parseModelRoute(req.Model, effDefaultProvider, configuredProviders)
	effProvider := provider
	if effProvider == "" {
		effProvider = effDefaultProvider
	}
	if err := validateChatControls(req, effProvider); err != nil {
		writeError(w, http.StatusBadRequest, "unsupported_parameter", err.Error(), "invalid_request_error")
		return
	}

	msgs, err := parseChatMessages(req.Messages)
	if err != nil {
		writeError(w, http.StatusBadRequest, chatMessageErrorCode(err), err.Error(), "invalid_request_error")
		return
	}
	tools, toolChoice, err := parseChatToolConfig(req.Tools, req.ToolChoice)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error(), "invalid_request_error")
		return
	}

	builder := p.wh.Text().Model(model).Messages(msgs...)
	if provider != "" {
		builder = builder.Using(provider)
	}
	builder = applyChatGenerationControls(builder, req)
	if len(req.Tools) > 0 {
		builder = builder.Tools(tools...)
	}
	if toolChoice != nil {
		builder = builder.ToolChoice(toolChoice)
	}
	if len(req.ResponseFormat) > 0 {
		effProvider := provider
		if effProvider == "" {
			effProvider = effDefaultProvider
		}
		if responseFormatUnsupported(effProvider) {
			writeError(w, http.StatusBadRequest, "unsupported_response_format",
				fmt.Sprintf("response_format is not yet supported through the proxy for the %q provider; use the SDK's structured output instead", effProvider),
				"invalid_request_error")
			return
		}
		var rf map[string]any
		if err := json.Unmarshal(req.ResponseFormat, &rf); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request_error",
				"invalid response_format: "+err.Error(), "invalid_request_error")
			return
		}
		builder = builder.ResponseFormat(rf)
	}

	if req.Stream {
		p.streamChat(w, r, builder, model)
		return
	}

	resp, err := builder.Generate(r.Context())
	if err != nil {
		p.logger.Error("text generation failed", "error", types.SafeErrorValue(err), "model", types.SafeLogString(req.Model))
		status, errType, clientMsg := upstreamErrorStatus(err)
		writeError(w, status, "upstream_error", clientMsg, errType)
		return
	}

	fr := string(normalizedFinishReason(resp.FinishReason))

	msg := &ChatMessage{Role: "assistant", Content: resp.Text, Refusal: resp.Refusal}
	if len(resp.ToolCalls) > 0 {
		msg.ToolCalls = fromWormholeToolCalls(resp.ToolCalls)
	}

	out := ChatCompletionResponse{
		ID:      fmt.Sprintf("wh-%s", resp.ID),
		Object:  "chat.completion",
		Created: resp.Created.Unix(),
		Model:   model,
		Choices: []ChatChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: &fr,
		}},
	}
	if resp.Usage != nil {
		out.Usage = toChatUsage(resp.Usage)
	}
	writeJSON(w, http.StatusOK, out)
}

func applyChatGenerationControls(builder *wormhole.TextRequestBuilder, req ChatCompletionRequest) *wormhole.TextRequestBuilder {
	if req.Temperature != nil {
		builder = builder.Temperature(float32(*req.Temperature))
	}
	if req.MaxCompletionTokens != nil {
		builder = builder.MaxTokens(*req.MaxCompletionTokens)
	} else if req.MaxTokens != nil {
		builder = builder.MaxTokens(*req.MaxTokens)
	}
	if req.TopP != nil {
		builder = builder.TopP(float32(*req.TopP))
	}
	if req.FrequencyPenalty != nil {
		builder = builder.FrequencyPenalty(float32(*req.FrequencyPenalty))
	}
	if req.PresencePenalty != nil {
		builder = builder.PresencePenalty(float32(*req.PresencePenalty))
	}
	if req.Seed != nil {
		builder = builder.Seed(*req.Seed)
	}
	if req.ParallelToolCalls != nil {
		builder = builder.ParallelToolCalls(*req.ParallelToolCalls)
	}
	if len(req.Stop) > 0 {
		builder = builder.Stop(req.Stop...)
	}
	return builder
}

func validateChatControls(req ChatCompletionRequest, provider string) error {
	if req.N != nil && *req.N != 1 {
		return fmt.Errorf("n=%d is unsupported; the proxy currently returns exactly one choice", *req.N)
	}
	if req.FrequencyPenalty != nil && (*req.FrequencyPenalty < -2 || *req.FrequencyPenalty > 2) {
		return fmt.Errorf("frequency_penalty must be between -2.0 and 2.0")
	}
	if req.PresencePenalty != nil && (*req.PresencePenalty < -2 || *req.PresencePenalty > 2) {
		return fmt.Errorf("presence_penalty must be between -2.0 and 2.0")
	}
	if provider == "anthropic" && (req.FrequencyPenalty != nil || req.PresencePenalty != nil || req.Seed != nil) {
		return fmt.Errorf("frequency_penalty, presence_penalty, and seed are unsupported for Anthropic")
	}
	if (provider == "gemini" || provider == "ollama") && req.ParallelToolCalls != nil {
		return fmt.Errorf("parallel_tool_calls is unsupported for %s", provider)
	}
	return nil
}

func chatMessageErrorCode(err error) string {
	switch {
	case strings.Contains(err.Error(), "image content parts"):
		return "unsupported_content_part"
	case strings.Contains(err.Error(), "unsupported message role"):
		return "unsupported_message_role"
	default:
		return "invalid_request_error"
	}
}
