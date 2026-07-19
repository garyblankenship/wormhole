package server

import (
	"net/http"

	"github.com/garyblankenship/wormhole/v2/types"
)

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
	format := types.EmbeddingEncodingFormat(req.EncodingFormat)
	if format != "" && format != types.EmbeddingEncodingFloat && format != types.EmbeddingEncodingBase64 {
		writeError(w, http.StatusBadRequest, "invalid_encoding_format",
			"encoding_format must be float or base64", "invalid_request_error")
		return
	}
	if format != "" {
		builder = builder.EncodingFormat(format)
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
		var value any = emb.Embedding
		if emb.Base64 != "" {
			value = emb.Base64
		}
		data = append(data, EmbeddingData{
			Object:    "embedding",
			Index:     emb.Index,
			Embedding: value,
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

func (p *proxy) handleRerank(w http.ResponseWriter, r *http.Request) {
	var req RerankRequest
	if err := decodeRequestBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json",
			"Failed to parse request body: "+err.Error(), "invalid_request_error")
		return
	}

	switch {
	case req.Model == "":
		writeError(w, http.StatusBadRequest, "model_required",
			"model is required", "invalid_request_error")
		return
	case req.Query == "":
		writeError(w, http.StatusBadRequest, "query_required",
			"query is required", "invalid_request_error")
		return
	case len(req.Documents) == 0:
		writeError(w, http.StatusBadRequest, "documents_required",
			"documents is required", "invalid_request_error")
		return
	}

	configuredProviders := p.wh.ConfiguredProviders()
	effDefaultProvider := effectiveDefaultProvider(p.defaultProvider, configuredProviders)
	provider, model := parseModelRoute(req.Model, effDefaultProvider, configuredProviders)

	builder := p.wh.Rerank().Model(model).Query(req.Query).Documents(req.Documents...)
	if provider != "" {
		builder = builder.Using(provider)
	}
	if req.TopN != nil {
		builder = builder.TopN(*req.TopN)
	}

	resp, err := builder.Generate(r.Context())
	if err != nil {
		p.logger.Error("rerank failed", "error", types.SafeErrorValue(err), "model", types.SafeLogString(req.Model))
		status, errType, clientMsg := upstreamErrorStatus(err)
		writeError(w, status, "upstream_error", clientMsg, errType)
		return
	}

	results := make([]RerankResult, 0, len(resp.Results))
	for _, result := range resp.Results {
		results = append(results, RerankResult{
			Index:          result.Index,
			RelevanceScore: result.RelevanceScore,
			Document:       RerankDocument{Text: result.Document},
		})
	}
	responseModel := resp.Model
	if responseModel == "" {
		responseModel = model
	}
	out := RerankResponse{
		ID:      resp.ID,
		Model:   responseModel,
		Results: results,
	}
	if resp.Usage != nil {
		out.Usage = &RerankUsage{TotalTokens: resp.Usage.TotalTokens}
	}
	writeJSON(w, http.StatusOK, out)
}
