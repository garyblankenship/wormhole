package wormhole

import (
	"github.com/garyblankenship/wormhole/v2/types"
)

// Quick provides quick access to factory methods
var Quick = NewSimpleFactory()

// QuickOpenAI creates an OpenAI client with minimal configuration
func QuickOpenAI(apiKey ...string) *Wormhole {
	return Quick.OpenAI(apiKey...)
}

// QuickAnthropic creates an Anthropic client with minimal configuration
func QuickAnthropic(apiKey ...string) *Wormhole {
	return Quick.Anthropic(apiKey...)
}

// QuickGemini creates a Gemini client with minimal configuration
func QuickGemini(apiKey ...string) *Wormhole {
	return Quick.Gemini(apiKey...)
}

// QuickOllama creates an Ollama client with minimal configuration
func QuickOllama(baseURL ...string) (*Wormhole, error) {
	return Quick.Ollama(baseURL...)
}

// QuickLMStudio creates an LMStudio client with minimal configuration
func QuickLMStudio(baseURL ...string) (*Wormhole, error) {
	return Quick.LMStudio(baseURL...)
}

// QuickLocalOpenAI creates a no-auth OpenAI-compatible local client.
func QuickLocalOpenAI(baseURL string, config ...types.ProviderConfig) (*Wormhole, error) {
	return Quick.LocalOpenAI(baseURL, config...)
}

// QuickGroq creates a Groq client with minimal configuration
func QuickGroq(apiKey ...string) *Wormhole {
	return Quick.Groq(apiKey...)
}

// QuickMistral creates a Mistral client with minimal configuration
func QuickMistral(apiKey ...string) *Wormhole {
	return Quick.Mistral(apiKey...)
}

// QuickOpenRouter creates an OpenRouter client with minimal configuration
// This provides INSTANT access to ALL 200+ OpenRouter models through dynamic model support
// No manual registration required - any model name works immediately
func QuickOpenRouter(apiKey ...string) (*Wormhole, error) {
	return Quick.OpenRouter(apiKey...)
}
