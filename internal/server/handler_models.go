package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

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
