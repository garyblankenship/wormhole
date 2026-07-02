package server

import (
	"strings"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

// knownProviderSet is initialized once from embedded provider profiles.
// Adding a provider to provider_profiles.json makes it available as a
// proxy route prefix without editing this file.
var knownProviderSet = func() map[string]bool {
	names := wormhole.KnownProviderNames()
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}()

// openRouterProviderName is OpenRouter's provider name in provider_profiles.json.
// OpenRouter model IDs are themselves "org/model" pairs (e.g. "openai/gpt-4o",
// "anthropic/claude-3.5-sonnet") that collide with wormhole's own provider
// prefixes -- when it's the default provider, a colliding prefix must stay on
// the model string and pass through, not get hijacked into direct routing.
const openRouterProviderName = "openrouter"

// parseModelRoute splits a model string like "anthropic/claude-sonnet-4-5"
// into (provider, model). If no known provider prefix, returns ("", fullModel).
// When defaultProvider is openrouter, only an explicit "openrouter/" prefix is
// stripped -- every other known-provider prefix is OpenRouter's own org/model
// naming convention and must reach OpenRouter intact instead of being hijacked
// into direct routing to that provider.
func parseModelRoute(model, defaultProvider string) (provider, resolved string) {
	idx := strings.IndexByte(model, '/')
	if idx < 0 {
		return "", model
	}
	prefix := model[:idx]
	if !knownProviderSet[prefix] {
		return "", model
	}
	if defaultProvider == openRouterProviderName && prefix != openRouterProviderName {
		return "", model
	}
	return prefix, model[idx+1:]
}
