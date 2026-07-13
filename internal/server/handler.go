package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	wormhole "github.com/garyblankenship/wormhole/pkg/wormhole"
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
	if len(req.Stop) > 0 {
		builder = builder.Stop(req.Stop...)
	}
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

func (p *proxy) streamChat(w http.ResponseWriter, r *http.Request, builder *wormhole.TextRequestBuilder, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported",
			"Streaming not supported", "api_error")
		return
	}

	stream, err := builder.Stream(r.Context())
	if err != nil {
		p.logger.Error("stream creation failed", "error", types.SafeErrorValue(err), "model", types.SafeLogString(model))
		status, errType, clientMsg := upstreamErrorStatus(err)
		writeError(w, status, "upstream_error", clientMsg, errType)
		return
	}

	id := fmt.Sprintf("wh-%d", time.Now().UnixNano())
	toolState := newStreamToolState()
	committed := false

	for chunk := range stream {
		if chunk.Error != nil {
			p.logger.Error("stream chunk error", "error", types.SafeErrorValue(chunk.Error))
			if !committed {
				status, errType, clientMsg := upstreamErrorStatus(chunk.Error)
				writeError(w, status, "upstream_error", clientMsg, errType)
				return
			}
			writeStreamError(w, flusher, chunk.Error)
			return
		}

		if !committed {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			flusher.Flush()
			committed = true
		}

		delta := &ChatMessage{Role: "assistant", Content: chunk.Content(), Refusal: chunk.Refusal}
		if tcs := toolState.delta(chunk); len(tcs) > 0 {
			delta.ToolCalls = tcs
		}
		chunkResp := ChatCompletionResponse{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []ChatChoice{{
				Index: 0,
				Delta: delta,
			}},
		}

		if chunk.FinishReason != nil {
			fr := string(normalizedFinishReason(*chunk.FinishReason))
			chunkResp.Choices[0].FinishReason = &fr
		}
		if chunk.Usage != nil {
			chunkResp.Usage = toChatUsage(chunk.Usage)
		}

		data, marshalErr := json.Marshal(chunkResp)
		if marshalErr != nil {
			p.logger.Error("failed to marshal chunk", "error", types.SafeErrorValue(marshalErr))
			break
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			p.logger.Error("failed to write stream chunk", "error", types.SafeErrorValue(err))
			break
		}
		flusher.Flush()
	}

	if !committed {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
	}

	if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
		p.logger.Error("failed to write stream terminator", "error", types.SafeErrorValue(err))
		return
	}
	flusher.Flush()
}

func toChatUsage(usage *types.Usage) *ChatUsage {
	if usage == nil {
		return nil
	}
	out := &ChatUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
	if usage.CacheReadTokens != 0 || usage.CacheWriteTokens != 0 {
		out.PromptTokensDetails = &ChatPromptTokenDetails{
			CachedTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		}
	}
	if usage.ReasoningTokens != 0 {
		out.CompletionTokensDetails = &ChatCompletionTokenDetails{ReasoningTokens: usage.ReasoningTokens}
	}
	return out
}

func normalizedFinishReason(reason types.FinishReason) types.FinishReason {
	switch reason {
	case types.FinishReasonStop, types.FinishReasonLength, types.FinishReasonToolCalls,
		types.FinishReasonContentFilter, types.FinishReasonOther:
		return reason
	default:
		return types.FinishReasonOther
	}
}

func writeStreamError(w http.ResponseWriter, flusher http.Flusher, err error) {
	_, errType, clientMsg := upstreamErrorStatus(err)
	payload := ErrorResponse{
		Error: ErrorDetail{
			Message: clientMsg,
			Type:    errType,
			Code:    "upstream_error",
		},
	}
	data, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func (p *proxy) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	var req EmbeddingRequest
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
	if len(req.Input) == 0 {
		writeError(w, http.StatusBadRequest, "input_required",
			"input is required", "invalid_request_error")
		return
	}

	configuredProviders := p.wh.ConfiguredProviders()
	effDefaultProvider := effectiveDefaultProvider(p.defaultProvider, configuredProviders)
	provider, model := parseModelRoute(req.Model, effDefaultProvider, configuredProviders)

	builder := p.wh.Embeddings().Model(model).Input([]string(req.Input)...)
	if provider != "" {
		builder = builder.Using(provider)
	}
	if req.Dimensions != nil {
		builder = builder.Dimensions(*req.Dimensions)
	}

	resp, err := builder.Generate(r.Context())
	if err != nil {
		p.logger.Error("embeddings failed", "error", types.SafeErrorValue(err), "model", types.SafeLogString(req.Model))
		status, errType, clientMsg := upstreamErrorStatus(err)
		writeError(w, status, "upstream_error", clientMsg, errType)
		return
	}

	data := make([]EmbeddingData, 0, len(resp.Embeddings))
	for _, emb := range resp.Embeddings {
		data = append(data, EmbeddingData{
			Object:    "embedding",
			Index:     emb.Index,
			Embedding: emb.Embedding,
		})
	}

	out := EmbeddingResponse{
		Object: "list",
		Data:   data,
		Model:  model,
	}
	if resp.Usage != nil {
		out.Usage = &EmbeddingUsage{
			PromptTokens: resp.Usage.PromptTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (p *proxy) handleListModels(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("client_version") {
		writeJSON(w, http.StatusOK, struct {
			Models []any `json:"models"`
		}{Models: []any{}})
		return
	}

	providers := mergeProviderNames(p.wh.ConfiguredProviders(), p.wh.ModelDiscoveryProviders())
	var entries []ModelEntry
	ts := time.Now().Unix()

	for _, prov := range providers {
		models, err := p.wh.ListAvailableModelsWithContext(r.Context(), prov)
		if err != nil {
			continue
		}
		for _, m := range models {
			entries = append(entries, ModelEntry{
				ID:      fmt.Sprintf("%s/%s", prov, m.ID),
				Object:  "model",
				Created: ts,
				OwnedBy: prov,
			})
		}
	}

	if entries == nil {
		entries = []ModelEntry{}
	}

	writeJSON(w, http.StatusOK, ModelListResponse{
		Object: "list",
		Data:   entries,
	})
}

func mergeProviderNames(groups ...[]string) []string {
	seen := make(map[string]bool)
	var providers []string
	for _, group := range groups {
		for _, provider := range group {
			if seen[provider] {
				continue
			}
			seen[provider] = true
			providers = append(providers, provider)
		}
	}
	return providers
}

func decodeRequestBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxProxyRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("request body must contain a single JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message, errType string) {
	writeJSON(w, status, ErrorResponse{
		Error: ErrorDetail{
			Message: message,
			Type:    errType,
			Code:    code,
		},
	})
}

// responseFormatUnsupported reports whether the proxy must reject response_format
// for a provider rather than pass it through. Anthropic and Gemini never read
// ResponseFormat on the text path (they drive structured output through separate
// mechanisms), and native Ollama's text path only accepts a narrow shape — so a
// raw passthrough would silently yield unstructured output. OpenAI and all
// OpenAI-Chat-compatible providers handle it correctly.
func responseFormatUnsupported(provider string) bool {
	switch provider {
	case "anthropic", "gemini", "ollama":
		return true
	default:
		return false
	}
}

// upstreamErrorStatus maps a provider error to an OpenAI-style HTTP status and
// error type. When err carries a *types.WormholeError (via errors.As), its
// StatusCode and Code drive the response so clients can distinguish a 429 rate
// limit from a 400 bad request from a 401 auth failure. Falls back to 502
// (bad gateway) + "api_error" when no structured error is present.
func upstreamErrorStatus(err error) (int, string, string) {
	whErr, ok := types.AsWormholeError(err)
	if !ok {
		return http.StatusBadGateway, "api_error", "upstream provider error"
	}

	errType := wormholeErrorType(whErr.Code)
	if whErr.StatusCode != 0 {
		return whErr.StatusCode, errType, upstreamClientMessage(errType)
	}
	// No upstream status (SDK-internal error). Map by code to the semantically
	// correct HTTP status instead of defaulting everything to 502 bad gateway.
	switch whErr.Code {
	case types.ErrorCodeAuth:
		return http.StatusUnauthorized, errType, upstreamClientMessage(errType)
	case types.ErrorCodeRateLimit:
		return http.StatusTooManyRequests, errType, upstreamClientMessage(errType)
	case types.ErrorCodeTimeout:
		return http.StatusGatewayTimeout, errType, upstreamClientMessage(errType)
	case types.ErrorCodeModel, types.ErrorCodeRequest, types.ErrorCodeValidation:
		return http.StatusBadRequest, errType, actionableInvalidRequestMessage(whErr)
	default:
		return http.StatusBadGateway, errType, upstreamClientMessage(errType)
	}
}

func upstreamClientMessage(errType string) string {
	switch errType {
	case "authentication_error":
		return "upstream authentication failed"
	case "rate_limit_error":
		return "upstream rate limit exceeded"
	case "invalid_request_error":
		return "upstream request rejected"
	default:
		return "upstream provider error"
	}
}

// wormholeErrorType maps a WormholeError code to an OpenAI-style error type string.
func wormholeErrorType(code types.ErrorCode) string {
	switch code {
	case types.ErrorCodeAuth:
		return "authentication_error"
	case types.ErrorCodeRateLimit:
		return "rate_limit_error"
	case types.ErrorCodeModel, types.ErrorCodeRequest, types.ErrorCodeValidation:
		return "invalid_request_error"
	default:
		return "api_error"
	}
}

func actionableInvalidRequestMessage(err *types.WormholeError) string {
	if err == nil {
		return "upstream request rejected"
	}
	switch {
	case err.Message == "" && err.Details == "":
		return "upstream request rejected"
	case err.Message == "":
		return err.Details
	case err.Details == "":
		return err.Message
	default:
		return err.Message + ": " + err.Details
	}
}
