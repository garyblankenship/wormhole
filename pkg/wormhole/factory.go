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
	key := f.getAPIKey(apiKey, "OPENAI_API_KEY")

	return New(
		WithDefaultProvider("openai"),
		WithOpenAI(key),
	)
}

// Anthropic creates a Wormhole client configured for Anthropic
func (f *SimpleFactory) Anthropic(apiKey ...string) *Wormhole {
	key := f.getAPIKey(apiKey, "ANTHROPIC_API_KEY")

	return New(
		WithDefaultProvider("anthropic"),
		WithAnthropic(key),
	)
}

// Gemini creates a Wormhole client configured for Google Gemini
func (f *SimpleFactory) Gemini(apiKey ...string) *Wormhole {
	key := f.getAPIKey(apiKey, "GEMINI_API_KEY", "GOOGLE_API_KEY")

	return New(
		WithDefaultProvider("gemini"),
		WithGemini(key),
	)
}

// Ollama creates a Wormhole client configured for Ollama
func (f *SimpleFactory) Ollama(baseURL ...string) (*Wormhole, error) {
	var url string
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = baseURL[0]
	} else if envURL := os.Getenv("OLLAMA_BASE_URL"); envURL != "" {
		url = envURL
	} else {
		return nil, fmt.Errorf("Ollama base URL is required: provide via parameter or OLLAMA_BASE_URL environment variable")
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
	key := f.getAPIKey(apiKey, "GROQ_API_KEY")

	return New(
		WithDefaultProvider("groq"),
		WithGroq(key),
	)
}

// Mistral creates a Wormhole client configured for Mistral
func (f *SimpleFactory) Mistral(apiKey ...string) *Wormhole {
	key := f.getAPIKey(apiKey, "MISTRAL_API_KEY")

	return New(
		WithDefaultProvider("mistral"),
		WithMistral(types.ProviderConfig{APIKey: key}),
	)
}

// LMStudio creates a Wormhole client configured for LMStudio
func (f *SimpleFactory) LMStudio(baseURL ...string) (*Wormhole, error) {
	var url string
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = baseURL[0]
	} else if envURL := os.Getenv("LMSTUDIO_BASE_URL"); envURL != "" {
		url = envURL
	} else {
		return nil, fmt.Errorf("LMStudio base URL is required: provide via parameter or LMSTUDIO_BASE_URL environment variable")
	}

	return New(
		WithDefaultProvider("lmstudio"),
		WithLMStudio(types.ProviderConfig{
			BaseURL:       url,
			DynamicModels: true, // Users can load any model in LMStudio
		}),
	), nil
}

// OpenRouter creates a Wormhole client configured for OpenRouter (multi-provider gateway)
func (f *SimpleFactory) OpenRouter(apiKey ...string) (*Wormhole, error) {
	key := f.getAPIKey(apiKey, "OPENROUTER_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("OpenRouter API key is required: provide via parameter or OPENROUTER_API_KEY environment variable")
	}

	return New(
		WithDefaultProvider("openrouter"),
		WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
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
	return WithMiddleware(middleware.CacheMiddleware(config))
}

// WithTimeout returns an option to add timeout middleware
func (f *SimpleFactory) WithTimeout(timeout time.Duration) Option {
	return WithMiddleware(middleware.TimeoutMiddleware(timeout))
}

// WithMetrics returns an option to add metrics tracking middleware and the metrics instance
func (f *SimpleFactory) WithMetrics() (Option, *middleware.Metrics) {
	metrics := middleware.NewMetrics()
	return WithMiddleware(middleware.MetricsMiddleware(metrics)), metrics
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
	return WithMiddleware(middleware.DebugLoggingMiddleware(logger))
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
