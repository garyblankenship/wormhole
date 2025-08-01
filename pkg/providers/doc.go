// Package providers contains implementations of various LLM provider integrations.
//
// Each provider package implements the types.Provider interface and handles the
// specific API requirements, authentication, and response transformations for
// that provider.
//
// Currently supported providers:
//   - OpenAI (GPT-3.5, GPT-4, DALL-E, Whisper)
//   - Anthropic (Claude 3 family)
//
// All providers share common functionality through the BaseProvider type, which
// handles HTTP requests, error handling, and common patterns.
package providers
