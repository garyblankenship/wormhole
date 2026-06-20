package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

	provider, model := parseModelRoute(req.Model)

	msgs := make([]types.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			if len(m.Content.Media) > 0 {
				writeError(w, http.StatusBadRequest, "unsupported_content_part",
					"image content parts are only supported on user messages", "invalid_request_error")
				return
			}
			msgs = append(msgs, types.NewSystemMessage(m.Content.Text))
		case "user":
			msgs = append(msgs, &types.UserMessage{
				Content: m.Content.Text,
				Media:   m.Content.Media,
			})
		case "assistant":
			if len(m.Content.Media) > 0 {
				writeError(w, http.StatusBadRequest, "unsupported_content_part",
					"image content parts are only supported on user messages", "invalid_request_error")
				return
			}
			msgs = append(msgs, types.NewAssistantMessage(m.Content.Text))
		case "tool":
			if len(m.Content.Media) > 0 {
				writeError(w, http.StatusBadRequest, "unsupported_content_part",
					"image content parts are only supported on user messages", "invalid_request_error")
				return
			}
			msgs = append(msgs, types.NewToolResultMessage(m.ToolCallID, m.Content.Text))
		default:
			if len(m.Content.Media) > 0 {
				writeError(w, http.StatusBadRequest, "unsupported_content_part",
					"image content parts are only supported on user messages", "invalid_request_error")
				return
			}
			msgs = append(msgs, types.NewUserMessage(m.Content.Text))
		}
	}

	builder := p.wh.Text().Model(model).Messages(msgs...)
	if provider != "" {
		builder = builder.Using(provider)
	}
	if req.Temperature != nil {
		builder = builder.Temperature(float32(*req.Temperature))
	}
	if req.MaxTokens != nil {
		builder = builder.MaxTokens(*req.MaxTokens)
	}
	if req.TopP != nil {
		builder = builder.TopP(float32(*req.TopP))
	}
	if len(req.Stop) > 0 {
		builder = builder.Stop(req.Stop...)
	}

	if req.Stream {
		p.streamChat(w, r, builder, model)
		return
	}

	resp, err := builder.Generate(r.Context())
	if err != nil {
		p.logger.Error("text generation failed", "error", err, "model", req.Model)
		status, errType := upstreamErrorStatus(err)
		writeError(w, status, "upstream_error", err.Error(), errType)
		return
	}

	fr := string(resp.FinishReason)
	if fr == "" {
		fr = "stop"
	}

	out := ChatCompletionResponse{
		ID:      fmt.Sprintf("wh-%s", resp.ID),
		Object:  "chat.completion",
		Created: resp.Created.Unix(),
		Model:   model,
		Choices: []ChatChoice{{
			Index:        0,
			Message:      &ChatMessage{Role: "assistant", Content: resp.Text},
			FinishReason: &fr,
		}},
	}
	if resp.Usage != nil {
		out.Usage = &ChatUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}
	writeJSON(w, http.StatusOK, out)
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
		p.logger.Error("stream creation failed", "error", err, "model", model)
		status, errType := upstreamErrorStatus(err)
		writeError(w, status, "upstream_error", err.Error(), errType)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	id := fmt.Sprintf("wh-%d", time.Now().UnixNano())

	for chunk := range stream {
		if chunk.Error != nil {
			p.logger.Error("stream chunk error", "error", chunk.Error)
			break
		}

		chunkResp := ChatCompletionResponse{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []ChatChoice{{
				Index: 0,
				Delta: &ChatMessage{Role: "assistant", Content: chunk.Content()},
			}},
		}

		if chunk.FinishReason != nil {
			fr := string(*chunk.FinishReason)
			chunkResp.Choices[0].FinishReason = &fr
		}
		if chunk.Usage != nil {
			chunkResp.Usage = &ChatUsage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}

		data, marshalErr := json.Marshal(chunkResp)
		if marshalErr != nil {
			p.logger.Error("failed to marshal chunk", "error", marshalErr)
			break
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			p.logger.Error("failed to write stream chunk", "error", err)
			break
		}
		flusher.Flush()
	}

	if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
		p.logger.Error("failed to write stream terminator", "error", err)
		return
	}
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

	provider, model := parseModelRoute(req.Model)

	builder := p.wh.Embeddings().Model(model).Input([]string(req.Input)...)
	if provider != "" {
		builder = builder.Using(provider)
	}

	resp, err := builder.Generate(r.Context())
	if err != nil {
		p.logger.Error("embeddings failed", "error", err, "model", req.Model)
		status, errType := upstreamErrorStatus(err)
		writeError(w, status, "upstream_error", err.Error(), errType)
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

// upstreamErrorStatus maps a provider error to an OpenAI-style HTTP status and
// error type. When err carries a *types.WormholeError (via errors.As), its
// StatusCode and Code drive the response so clients can distinguish a 429 rate
// limit from a 400 bad request from a 401 auth failure. Falls back to 502
// (bad gateway) + "api_error" when no structured error is present or StatusCode
// is unset.
func upstreamErrorStatus(err error) (int, string) {
	whErr, ok := types.AsWormholeError(err)
	if !ok {
		return http.StatusBadGateway, "api_error"
	}

	errType := wormholeErrorType(whErr.Code)

	if whErr.StatusCode != 0 {
		return whErr.StatusCode, errType
	}
	return http.StatusBadGateway, errType
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
