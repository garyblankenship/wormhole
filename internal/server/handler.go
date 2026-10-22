package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
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
	req, ok := decodeChatCompletion(w, r)
	if !ok {
		return
	}
	if !requireChatBasics(w, req) {
		return
	}

	route := p.resolveModelRoute(req.Model)
	if err := validateChatControls(req, route.effectiveProvider); err != nil {
		writeError(w, http.StatusBadRequest, "unsupported_parameter", err.Error(), "invalid_request_error")
		return
	}
	templateKwargs, ok := p.prepareTemplateKwargs(w, req, route.effectiveProvider)
	if !ok {
		return
	}
	if !p.enforceWormholeExtensionGates(w, req, route) {
		return
	}
	inputs, ok := parseChatInputs(w, req, route.effectiveProvider)
	if !ok {
		return
	}
	builder := p.newChatBuilder(req, route, inputs, templateKwargs)
	builder, ok = p.applyChatResponseFormat(w, builder, req, route)
	if !ok {
		return
	}

	if req.Stream {
		p.streamChat(w, r, builder, route.model, req.Wormhole)
		return
	}

	resp, err := builder.Generate(r.Context())
	if err != nil {
		p.logger.Error("text generation failed", "error", types.SafeErrorValue(err), "model", types.SafeLogString(req.Model))
		writeUpstreamError(w, err)
		return
	}

	out := chatCompletionResponse(resp, route.model, req.Wormhole, time.Now())
	forwardResponseMetadata(w, resp.ResponseMetadata)
	writeJSON(w, http.StatusOK, out)
}

func applyChatGenerationControls(builder *wormhole.TextRequestBuilder, req ChatCompletionRequest) *wormhole.TextRequestBuilder {
	if req.ReasoningEffort != nil {
		builder = builder.Reasoning(types.Reasoning{Effort: *req.ReasoningEffort})
	}
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
	if req.Thinking != nil && chatTemplateKwargsUnsupported(provider) {
		return fmt.Errorf("thinking is unsupported for %s", provider)
	}
	if req.ReasoningEffort != nil {
		switch *req.ReasoningEffort {
		case types.ReasoningEffortNone, types.ReasoningEffortLow, types.ReasoningEffortMedium, types.ReasoningEffortHigh:
		default:
			return fmt.Errorf("unsupported reasoning_effort %q", *req.ReasoningEffort)
		}
	}
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
