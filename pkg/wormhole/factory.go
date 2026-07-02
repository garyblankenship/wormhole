package wormhole

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// SimpleFactory provides Laravel-inspired factory methods for common use cases
type SimpleFactory struct{}

// NewSimpleFactory creates a new SimpleFactory instance
func NewSimpleFactory() *SimpleFactory {
	return &SimpleFactory{}
}

// OpenAI creates a Wormhole client configured for OpenAI
func (f *SimpleFactory) OpenAI(apiKey ...string) *Wormhole {
	key := f.getProfileAPIKey(apiKey, providerOpenAI)

	return New(
		WithDefaultProvider("openai"),
		WithOpenAI(key),
	)
}

// Anthropic creates a Wormhole client configured for Anthropic
func (f *SimpleFactory) Anthropic(apiKey ...string) *Wormhole {
	key := f.getProfileAPIKey(apiKey, providerAnthropic)

	return New(
		WithDefaultProvider("anthropic"),
		WithAnthropic(key),
	)
}

// Gemini creates a Wormhole client configured for Google Gemini
func (f *SimpleFactory) Gemini(apiKey ...string) *Wormhole {
	key := f.getProfileAPIKey(apiKey, providerGemini)

	return New(
		WithDefaultProvider("gemini"),
		WithGemini(key),
	)
}

// Ollama creates a Wormhole client configured for Ollama
func (f *SimpleFactory) Ollama(baseURL ...string) (*Wormhole, error) {
	url, ok := f.getRequiredProfileBaseURL(baseURL, providerOllama)
	if !ok {
		return nil, fmt.Errorf("Ollama base URL is required: provide via parameter or %s environment variable", primaryBaseURLEnv(providerOllama))
	}

	return New(
		WithDefaultProvider("ollama"),
		WithOllama(types.ProviderConfig{
			BaseURL:       url,
			DynamicModels: true, // Users can load any model in Ollama
		}),
	), nil
}

// Groq creates a Wormhole client configured for Groq
func (f *SimpleFactory) Groq(apiKey ...string) *Wormhole {
	key := f.getProfileAPIKey(apiKey, "groq")

	return New(
		WithDefaultProvider("groq"),
		WithGroq(key),
	)
}

// Mistral creates a Wormhole client configured for Mistral
func (f *SimpleFactory) Mistral(apiKey ...string) *Wormhole {
	key := f.getProfileAPIKey(apiKey, "mistral")

	return New(
		WithDefaultProvider("mistral"),
		WithMistral(types.ProviderConfig{APIKey: key}),
	)
}

// LMStudio creates a Wormhole client configured for LMStudio
func (f *SimpleFactory) LMStudio(baseURL ...string) (*Wormhole, error) {
	url, ok := f.getRequiredProfileBaseURL(baseURL, "lmstudio")
	if !ok {
		return nil, fmt.Errorf("LMStudio base URL is required: provide via parameter or %s environment variable", primaryBaseURLEnv("lmstudio"))
	}

	return New(
		WithDefaultProvider("lmstudio"),
		WithLMStudio(types.ProviderConfig{
			BaseURL:       url,
			DynamicModels: true, // Users can load any model in LMStudio
		}),
	), nil
}

// LocalOpenAI creates a no-auth OpenAI-compatible local client. The base URL
// should include the compatible API root, usually http://host:port/v1.
func (f *SimpleFactory) LocalOpenAI(baseURL string, config ...types.ProviderConfig) (*Wormhole, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("local OpenAI-compatible base URL is required")
	}
	return New(WithLocalOpenAI(baseURL, config...)), nil
}

// OpenRouter creates a Wormhole client configured for OpenRouter (multi-provider gateway)
func (f *SimpleFactory) OpenRouter(apiKey ...string) (*Wormhole, error) {
	key := f.getProfileAPIKey(apiKey, providerOpenRouter)
	if key == "" {
		return nil, fmt.Errorf("OpenRouter API key is required: provide via parameter or %s environment variable", primaryAPIKeyEnv(providerOpenRouter))
	}

	return New(
		WithDefaultProvider("openrouter"),
		WithProfiledOpenAICompatible("openrouter", types.ProviderConfig{
			APIKey:        key,
			DynamicModels: true, // Enable all 200+ OpenRouter models without registry validation
		}),
	), nil
}

// WithRateLimit returns an option to add rate limiting middleware
func (f *SimpleFactory) WithRateLimit(requestsPerSecond int) Option {
	return WithMiddleware(middleware.RateLimitMiddleware(requestsPerSecond))
}

// WithCircuitBreaker returns an option to add circuit breaker middleware
func (f *SimpleFactory) WithCircuitBreaker(threshold int, timeout time.Duration) Option {
	return WithMiddleware(middleware.CircuitBreakerMiddleware(threshold, timeout))
}

// WithCache returns an option to add caching middleware
func (f *SimpleFactory) WithCache(ttl time.Duration) Option {
	cache := middleware.NewMemoryCache(1000)
	config := middleware.CacheConfig{
		Cache: cache,
		TTL:   ttl,
	}
	return func(c *Config) {
		c.Closers = append(c.Closers, cache)
		WithMiddleware(middleware.CacheMiddleware(config))(c)
	}
}

// WithTimeout returns an option to add timeout middleware
func (f *SimpleFactory) WithTimeout(timeout time.Duration) Option {
	return WithProviderMiddleware(middleware.NewTypedTimeoutMiddleware(timeout))
}

// WithMetrics returns an option to add metrics tracking middleware and the metrics instance
func (f *SimpleFactory) WithMetrics() (Option, *middleware.TypedMetrics) {
	metrics := middleware.NewTypedMetrics()
	return WithProviderMiddleware(middleware.NewTypedMetricsMiddleware(metrics)), metrics
}

// WithLogging returns an option to add basic logging middleware
func (f *SimpleFactory) WithLogging(logger types.Logger) Option {
	return WithMiddleware(middleware.LoggingMiddleware(logger))
}

// WithDetailedLogging returns an option to add detailed logging middleware with configuration
func (f *SimpleFactory) WithDetailedLogging(logger types.Logger) Option {
	config := middleware.DefaultLoggingConfig(logger)
	return WithMiddleware(middleware.DetailedLoggingMiddleware(config))
}

// WithDebugLogging returns an option to add debug logging middleware
func (f *SimpleFactory) WithDebugLogging(logger types.Logger) Option {
	return WithProviderMiddleware(middleware.NewDebugTypedLoggingMiddleware(logger))
}

// getAPIKey retrieves API key from provided value or environment variables
func (f *SimpleFactory) getAPIKey(provided []string, envVars ...string) string {
	// Check if API key was provided directly
	if len(provided) > 0 && provided[0] != "" {
		return provided[0]
	}

	// Check environment variables
	for _, env := range envVars {
		if key := os.Getenv(env); key != "" {
			return key
		}
	}

	return ""
}

func (f *SimpleFactory) getProfileAPIKey(provided []string, provider string) string {
	if len(provided) > 0 && provided[0] != "" {
		return provided[0]
	}
	profile, ok := providerProfile(provider)
	if !ok {
		return ""
	}
	return configuredAPIKey(profile)
}

func (f *SimpleFactory) getRequiredProfileBaseURL(provided []string, provider string) (string, bool) {
	if len(provided) > 0 && provided[0] != "" {
		return provided[0], true
	}
	profile, ok := providerProfile(provider)
	if !ok {
		return "", false
	}
	if profile.BaseURLEnv != "" {
		if value := os.Getenv(profile.BaseURLEnv); value != "" {
			return value, true
		}
	}
	return "", false
}

func primaryAPIKeyEnv(provider string) string {
	profile, ok := providerProfile(provider)
	if !ok || len(profile.APIKeyEnv) == 0 {
		return provider + "_API_KEY"
	}
	return profile.APIKeyEnv[0]
}

func primaryBaseURLEnv(provider string) string {
	profile, ok := providerProfile(provider)
	if !ok || profile.BaseURLEnv == "" {
		return provider + "_BASE_URL"
	}
	return profile.BaseURLEnv
}

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

// ==================== Ultra-Quick One-Liners ====================
// These functions provide the absolute minimum path from idea to working code.

// QuickText generates text with minimal configuration.
// This is the fastest path to a working LLM call - perfect for scripts, demos, and prototyping.
//
// Example:
//
//	response, err := wormhole.QuickText("gpt-4o", "What is Go?", os.Getenv("OPENAI_API_KEY"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(response.Text)
func QuickText(model, prompt, apiKey string) (*types.TextResponse, error) {
	return QuickTextWithContext(context.Background(), model, prompt, apiKey)
}

// QuickTextWithContext generates text with context support for cancellation/timeout.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	response, err := wormhole.QuickTextWithContext(ctx, "gpt-4o", "What is Go?", apiKey)
func QuickTextWithContext(ctx context.Context, model, prompt, apiKey string) (*types.TextResponse, error) {
	return QuickOpenAI(apiKey).Text().
		Model(model).
		Prompt(prompt).
		Generate(ctx)
}

// QuickChat generates a response in a conversation with system context.
// This is useful for chat-like interactions where you need a system prompt.
//
// Example:
//
//	response, err := wormhole.QuickChat(
//	    "gpt-4o",
//	    "You are a helpful coding assistant.",
//	    "How do I read a file in Go?",
//	    os.Getenv("OPENAI_API_KEY"),
//	)
func QuickChat(model, systemPrompt, userMessage, apiKey string) (*types.TextResponse, error) {
	return QuickChatWithContext(context.Background(), model, systemPrompt, userMessage, apiKey)
}

// QuickChatWithContext generates a chat response with context support.
func QuickChatWithContext(ctx context.Context, model, systemPrompt, userMessage, apiKey string) (*types.TextResponse, error) {
	return QuickOpenAI(apiKey).Text().
		Model(model).
		SystemPrompt(systemPrompt).
		Prompt(userMessage).
		Generate(ctx)
}

// QuickStream streams text generation for real-time output.
//
// Example:
//
//	stream, err := wormhole.QuickStream("gpt-4o", "Write a haiku", apiKey)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for chunk := range stream {
//	    fmt.Print(chunk.Text)
//	}
func QuickStream(model, prompt, apiKey string) (<-chan types.TextChunk, error) {
	return QuickStreamWithContext(context.Background(), model, prompt, apiKey)
}

// QuickStreamWithContext streams text with context support for cancellation.
func QuickStreamWithContext(ctx context.Context, model, prompt, apiKey string) (<-chan types.TextChunk, error) {
	return QuickOpenAI(apiKey).Text().
		Model(model).
		Prompt(prompt).
		Stream(ctx)
}
