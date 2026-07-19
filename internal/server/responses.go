package server

import (
	"net/http"

	"github.com/garyblankenship/wormhole/v2/types"
)

func (p *proxy) handleResponses(w http.ResponseWriter, r *http.Request) {
	var req responsesRequest
	if err := decodeRequestBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Failed to parse request body: "+err.Error(), "invalid_request_error")
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model_required", "model is required", "invalid_request_error")
		return
	}
	if req.Input.Text == "" && len(req.Input.Items) == 0 {
		writeError(w, http.StatusBadRequest, "input_required", "input is required", "invalid_request_error")
		return
	}
	if req.Store || req.PreviousResponseID != "" {
		writeError(w, http.StatusBadRequest, "unsupported_state", "store and previous_response_id are not supported by the stateless proxy", "invalid_request_error")
		return
	}

	execution, err := p.responsesBuilder(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), "invalid_request_error")
		return
	}
	if req.Stream {
		p.streamResponses(w, r, execution)
		return
	}

	resp, err := execution.builder.Generate(r.Context())
	if err != nil {
		p.logger.Error("responses generation failed", "error", types.SafeErrorValue(err), "model", types.SafeLogString(req.Model))
		status, errType, clientMsg := upstreamErrorStatus(err)
		writeError(w, status, "upstream_error", clientMsg, errType)
		return
	}
	writeJSON(w, http.StatusOK, completedResponsesEnvelope(resp, execution.model, execution.customTools))
}

func (p *proxy) responsesBuilder(req responsesRequest) (responsesExecution, error) {
	messages, err := responsesMessages(req)
	if err != nil {
		return responsesExecution{}, err
	}
	configuredProviders := p.wh.ConfiguredProviders()
	effDefaultProvider := effectiveDefaultProvider(p.defaultProvider, configuredProviders)
	provider, model := parseModelRoute(req.Model, effDefaultProvider, configuredProviders)

	builder := p.wh.Text().Model(model).Messages(messages...)
	toolSelection, err := parseResponsesToolChoice(req.ToolChoice)
	if err != nil {
		return responsesExecution{}, err
	}
	if provider != "" {
		builder = builder.Using(provider)
	}
	if req.Temperature != nil {
		builder = builder.Temperature(float32(*req.Temperature))
	}
	if req.TopP != nil {
		builder = builder.TopP(float32(*req.TopP))
	}
	if req.MaxOutputTokens != nil {
		builder = builder.MaxTokens(*req.MaxOutputTokens)
	}
	tools, customTools, err := translateResponsesTools(req.Tools, toolSelection)
	if err != nil {
		return responsesExecution{}, err
	}
	if len(tools) > 0 {
		builder = builder.Tools(tools...)
	}
	if toolSelection.choice != nil {
		builder = builder.ToolChoice(toolSelection.choice)
	}
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		targetProvider := provider
		if targetProvider == "" {
			targetProvider = effDefaultProvider
		}
		if targetProvider == "zai" {
			thinkingType := "enabled"
			if req.Reasoning.Effort == "none" {
				thinkingType = "disabled"
			}
			builder = builder.ProviderOptions(map[string]any{"thinking": map[string]any{"type": thinkingType}})
		} else if req.Reasoning.Effort != "none" {
			builder = builder.Reasoning(types.Reasoning{Effort: types.ReasoningEffort(req.Reasoning.Effort)})
		}
	}
	return responsesExecution{builder: builder, model: model, customTools: customTools}, nil
}
