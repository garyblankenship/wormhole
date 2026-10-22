package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
)

// WormholeRequest selects additive provider-bridge controls. Unknown fields are
// rejected here without changing OpenAI's permissive top-level request parsing.
type WormholeRequest struct {
	Include        []string `json:"include,omitempty"`
	StructuredMode string   `json:"structured_mode,omitempty"`
	CachePrewarm   bool     `json:"cache_prewarm,omitempty"`
}

func (e *WormholeRequest) UnmarshalJSON(data []byte) error {
	type plain WormholeRequest
	var value plain
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("wormhole must be an object")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("invalid wormhole extension: %w", err)
	}
	for _, name := range value.Include {
		switch name {
		case "response_metadata", "usage_details", "sources", "citations", "warnings":
		default:
			return fmt.Errorf("unknown wormhole.include value %q", name)
		}
	}
	if value.StructuredMode != "" && value.StructuredMode != "tools" {
		return fmt.Errorf("wormhole.structured_mode must be tools")
	}
	*e = WormholeRequest(value)
	return nil
}

func extensionSnapshot(e *WormholeRequest, metadata *types.ResponseMetadata, usage *types.Usage, sources []types.Source, citations []types.Citation, warnings []types.ResponseWarning) map[string]any {
	if e == nil || len(e.Include) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, name := range e.Include {
		switch name {
		case "response_metadata":
			if metadata != nil {
				filtered := types.CloneResponseMetadata(metadata)
				for header := range filtered.Headers {
					if !safeProxyHeader(header) {
						delete(filtered.Headers, header)
					}
				}
				out[name] = filtered
			}
		case "usage_details":
			if usage != nil {
				out[name] = usage
			}
		case "sources":
			out[name] = sources
		case "citations":
			out[name] = citations
		case "warnings":
			out[name] = warnings
		}
	}
	return out
}

// Forward only the fixed public correlation/rate-limit surface; SDK-configured
// custom response headers never become proxy response headers.
func forwardResponseMetadata(w http.ResponseWriter, metadata *types.ResponseMetadata) {
	if metadata == nil {
		return
	}
	if metadata.RequestID != "" {
		w.Header().Set("X-Wormhole-Upstream-Request-ID", metadata.RequestID)
	}
	for name, values := range metadata.Headers {
		if !safeProxyHeader(name) {
			continue
		}
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
}

func safeProxyHeader(name string) bool {
	name = strings.ToLower(name)
	switch name {
	case "x-request-id", "request-id", "anthropic-request-id", "x-goog-request-id", "retry-after",
		"x-ratelimit-limit-requests", "x-ratelimit-limit-tokens", "x-ratelimit-remaining-requests", "x-ratelimit-remaining-tokens", "x-ratelimit-reset-requests", "x-ratelimit-reset-tokens",
		"anthropic-ratelimit-requests-limit", "anthropic-ratelimit-requests-remaining", "anthropic-ratelimit-requests-reset",
		"anthropic-ratelimit-tokens-limit", "anthropic-ratelimit-tokens-remaining", "anthropic-ratelimit-tokens-reset",
		"anthropic-ratelimit-input-tokens-limit", "anthropic-ratelimit-input-tokens-remaining", "anthropic-ratelimit-input-tokens-reset",
		"anthropic-ratelimit-output-tokens-limit", "anthropic-ratelimit-output-tokens-remaining", "anthropic-ratelimit-output-tokens-reset":
		return true
	default:
		return false
	}
}

func bindFileReferences(messages []types.Message, provider string) {
	for _, message := range messages {
		if user, ok := message.(*types.UserMessage); ok {
			for _, media := range user.Media {
				if ref, ok := media.(*types.FileReferenceMedia); ok && ref.Provider == "" {
					ref.Provider = provider
				}
			}
		}
	}
}

// nativeChatAnnotations preserves provider URL annotations that the standard
// Chat Completions shape can express, without guessing normalized offsets.
func nativeChatAnnotations(citations []types.Citation) []map[string]any {
	var out []map[string]any
	for _, citation := range citations {
		if citation.Raw["type"] != "url_citation" {
			continue
		}
		if _, ok := citation.Raw["url_citation"]; ok {
			out = append(out, citation.Raw)
			continue
		}
		value := map[string]any{}
		for key, v := range citation.Raw {
			if key != "type" {
				value[key] = v
			}
		}
		out = append(out, map[string]any{"type": "url_citation", "url_citation": value})
	}
	return out
}
