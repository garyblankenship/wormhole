package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

// decodeChatCompletion decodes the chat request body, writing the wire error itself.
func decodeChatCompletion(w http.ResponseWriter, r *http.Request) (ChatCompletionRequest, bool) {
	var req ChatCompletionRequest
	if err := decodeRequestBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json",
			"Failed to parse request body: "+err.Error(), "invalid_request_error")
		return ChatCompletionRequest{}, false
	}
	return req, true
}

// requireChatBasics enforces the required model and messages fields.
func requireChatBasics(w http.ResponseWriter, req ChatCompletionRequest) bool {
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model_required",
			"model is required", "invalid_request_error")
		return false
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages_required",
			"messages is required", "invalid_request_error")
		return false
	}
	return true
}

// prepareTemplateKwargs parses and provider-gates chat_template_kwargs.
func (p *proxy) prepareTemplateKwargs(w http.ResponseWriter, req ChatCompletionRequest, provider string) (map[string]any, bool) {
	templateKwargs, err := parseChatTemplateKwargs(req.ChatTemplateKwargs)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error(), "invalid_request_error")
		return nil, false
	}
	if templateKwargs != nil && chatTemplateKwargsUnsupported(provider) {
		writeError(w, http.StatusBadRequest, "unsupported_parameter",
			fmt.Sprintf("chat_template_kwargs is unsupported for the %q provider; it is an OpenAI-compatible chat-template extension", provider),
			"invalid_request_error")
		return nil, false
	}
	return templateKwargs, true
}

// enforceWormholeExtensionGates validates wormhole extension preconditions in
// wire order: response prewarming first, then structured tools formatting.
func (p *proxy) enforceWormholeExtensionGates(w http.ResponseWriter, req ChatCompletionRequest, route resolvedModelRoute) bool {
	if req.Wormhole == nil {
		return true
	}
	if req.Wormhole.CachePrewarm && !p.wh.ProviderUsesResponses(route.effectiveProvider) {
		writeError(w, http.StatusBadRequest, "unsupported_parameter",
			"cache_prewarm requires an OpenAI Responses route", "invalid_request_error")
		return false
	}
	if req.Wormhole.StructuredMode == "tools" {
		if ok, reason := validateStructuredToolsFormat(req.ResponseFormat); !ok {
			writeError(w, http.StatusBadRequest, "invalid_request_error", reason, "invalid_request_error")
			return false
		}
	}
	return true
}

// validateStructuredToolsFormat requires a non-strict schema response_format.
func validateStructuredToolsFormat(raw json.RawMessage) (bool, string) {
	var format struct {
		Type       string `json:"type"`
		JSONSchema struct {
			Strict bool           `json:"strict"`
			Schema map[string]any `json:"schema"`
		} `json:"json_schema"`
	}
	if err := json.Unmarshal(raw, &format); err != nil || format.Type != "json_schema" || format.JSONSchema.Schema == nil {
		return false, "structured_mode tools requires a schema response_format"
	}
	if format.JSONSchema.Strict {
		return false, "structured_mode tools conflicts with strict:true"
	}
	return true, ""
}

type chatInputs struct {
	messages   []types.Message
	tools      []types.Tool
	toolChoice *types.ToolChoice
}

// parseChatInputs decodes messages, binds file references, and parses tools.
func parseChatInputs(w http.ResponseWriter, req ChatCompletionRequest, provider string) (chatInputs, bool) {
	msgs, err := parseChatMessages(req.Messages)
	if err != nil {
		writeError(w, http.StatusBadRequest, chatMessageErrorCode(err), err.Error(), "invalid_request_error")
		return chatInputs{}, false
	}
	bindFileReferences(msgs, provider)
	tools, toolChoice, err := parseChatToolConfig(req.Tools, req.ToolChoice)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error(), "invalid_request_error")
		return chatInputs{}, false
	}
	return chatInputs{messages: msgs, tools: tools, toolChoice: toolChoice}, true
}

// newChatBuilder assembles the text builder in wire-request order.
func (p *proxy) newChatBuilder(req ChatCompletionRequest, route resolvedModelRoute, inputs chatInputs, templateKwargs map[string]any) *wormhole.TextRequestBuilder {
	builder := p.wh.Text().Model(route.model).Messages(inputs.messages...)
	if route.provider != "" {
		builder = builder.Using(route.provider)
	}
	builder = applyChatGenerationControls(builder, req)
	if providerOptions := chatProviderOptions(req, templateKwargs); len(providerOptions) > 0 {
		builder = builder.ProviderOptions(providerOptions)
	}
	if len(req.Tools) > 0 {
		builder = builder.Tools(inputs.tools...)
	}
	if inputs.toolChoice != nil {
		builder = builder.ToolChoice(inputs.toolChoice)
	}
	return builder
}

// chatProviderOptions merges proxy extension controls into provider options.
func chatProviderOptions(req ChatCompletionRequest, templateKwargs map[string]any) map[string]any {
	providerOptions := map[string]any{}
	if req.Wormhole != nil {
		if req.Wormhole.CachePrewarm {
			providerOptions["max_output_tokens"] = 0
		}
		if req.Wormhole.StructuredMode != "" {
			providerOptions["structured_mode"] = req.Wormhole.StructuredMode
		}
	}
	if req.Thinking != nil {
		providerOptions["thinking"] = req.Thinking
	}
	if templateKwargs != nil {
		// Merge only this nested field: request-level provider options replace
		// same-named top-level body keys, so nothing else may pass through here.
		providerOptions["chat_template_kwargs"] = templateKwargs
	}
	return providerOptions
}

// applyChatResponseFormat attaches response_format after provider gating.
func (p *proxy) applyChatResponseFormat(w http.ResponseWriter, builder *wormhole.TextRequestBuilder, req ChatCompletionRequest, route resolvedModelRoute) (*wormhole.TextRequestBuilder, bool) {
	if len(req.ResponseFormat) == 0 {
		return builder, true
	}
	if responseFormatUnsupported(route.effectiveProvider) && (req.Wormhole == nil || req.Wormhole.StructuredMode != "tools") {
		writeError(w, http.StatusBadRequest, "unsupported_response_format",
			fmt.Sprintf("response_format is not yet supported through the proxy for the %q provider; use the SDK's structured output instead", route.effectiveProvider),
			"invalid_request_error")
		return builder, false
	}
	var rf map[string]any
	if err := json.Unmarshal(req.ResponseFormat, &rf); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error",
			"invalid response_format: "+err.Error(), "invalid_request_error")
		return builder, false
	}
	return builder.ResponseFormat(rf), true
}
