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

// parseModelRoute splits a model string like "anthropic/claude-sonnet-4-5"
// into (provider, model). If no known provider prefix, returns ("", fullModel).
func parseModelRoute(model string) (provider, resolved string) {
	idx := strings.IndexByte(model, '/')
	if idx < 0 {
		return "", model
	}
	prefix := model[:idx]
	if knownProviderSet[prefix] {
		return prefix, model[idx+1:]
	}
	return "", model
}
