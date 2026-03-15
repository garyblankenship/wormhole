package server

import "strings"

// knownProviders lists provider names recognized for prefix routing.
var knownProviders = map[string]bool{
	"openai":     true,
	"anthropic":  true,
	"gemini":     true,
	"ollama":     true,
	"openrouter": true,
	"groq":       true,
	"mistral":    true,
	"lmstudio":   true,
	"vllm":       true,
}

// parseModelRoute splits a model string like "anthropic/claude-sonnet-4-5"
// into (provider, model). If no known provider prefix, returns ("", fullModel).
func parseModelRoute(model string) (provider, resolved string) {
	idx := strings.IndexByte(model, '/')
	if idx < 0 {
		return "", model
	}
	prefix := model[:idx]
	if knownProviders[prefix] {
		return prefix, model[idx+1:]
	}
	return "", model
}
