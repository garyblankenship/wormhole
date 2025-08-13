package wormhole

import (
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

	config := Config{
		DefaultProvider: "openai",
		Providers: map[string]types.ProviderConfig{
			"openai": {
				APIKey: key,
			},
		},
	}

	return New(config)
}

// Anthropic creates a Wormhole client configured for Anthropic
func (f *SimpleFactory) Anthropic(apiKey ...string) *Wormhole {
	key := f.getAPIKey(apiKey, "ANTHROPIC_API_KEY")

	config := Config{
		DefaultProvider: "anthropic",
		Providers: map[string]types.ProviderConfig{
			"anthropic": {
				APIKey: key,
			},
		},
	}

	return New(config)
}

// Gemini creates a Wormhole client configured for Google Gemini
func (f *SimpleFactory) Gemini(apiKey ...string) *Wormhole {
	key := f.getAPIKey(apiKey, "GEMINI_API_KEY", "GOOGLE_API_KEY")

	config := Config{
		DefaultProvider: "gemini",
		Providers: map[string]types.ProviderConfig{
			"gemini": {
				APIKey: key,
			},
		},
	}

	return New(config)
}

// Groq creates a Wormhole client configured for Groq
func (f *SimpleFactory) Groq(apiKey ...string) *Wormhole {
	key := f.getAPIKey(apiKey, "GROQ_API_KEY")

	config := Config{
		DefaultProvider: "groq",
		Providers: map[string]types.ProviderConfig{
			"groq": {
				APIKey: key,
			},
		},
	}

	return New(config)
}

// Mistral creates a Wormhole client configured for Mistral
func (f *SimpleFactory) Mistral(apiKey ...string) *Wormhole {
	key := f.getAPIKey(apiKey, "MISTRAL_API_KEY")

	config := Config{
		DefaultProvider: "mistral",
		Providers: map[string]types.ProviderConfig{
			"mistral": {
				APIKey: key,
			},
		},
	}

	return New(config)
}

// Ollama creates a Wormhole client configured for Ollama
func (f *SimpleFactory) Ollama(baseURL ...string) *Wormhole {
	var url string
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = baseURL[0]
	} else if envURL := os.Getenv("OLLAMA_BASE_URL"); envURL != "" {
		url = envURL
	} else {
		panic("Ollama base URL is required: provide via parameter or OLLAMA_BASE_URL environment variable")
	}

	config := Config{
		DefaultProvider: "ollama",
		Providers: map[string]types.ProviderConfig{
			"ollama": {
				BaseURL: url,
			},
		},
	}

	return New(config)
}

// LMStudio creates a Wormhole client configured for LMStudio
func (f *SimpleFactory) LMStudio(baseURL ...string) *Wormhole {
	var url string
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = baseURL[0]
	} else if envURL := os.Getenv("LMSTUDIO_BASE_URL"); envURL != "" {
		url = envURL
	} else {
		panic("LMStudio base URL is required: provide via parameter or LMSTUDIO_BASE_URL environment variable")
	}

	config := Config{
		DefaultProvider: "lmstudio",
		Providers: map[string]types.ProviderConfig{
			"lmstudio": {
				BaseURL: url,
			},
		},
	}

	p := New(config)
	return p.WithLMStudio(config.Providers["lmstudio"])
}

// WithRateLimit adds rate limiting middleware
func (f *SimpleFactory) WithRateLimit(wormhole *Wormhole, requestsPerSecond int) *Wormhole {
	return wormhole.Use(middleware.RateLimitMiddleware(requestsPerSecond))
}

// WithRetry adds retry middleware with exponential backoff
func (f *SimpleFactory) WithRetry(wormhole *Wormhole, maxRetries int) *Wormhole {
	config := middleware.RetryConfig{
		MaxRetries:   maxRetries,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	}
	return wormhole.Use(middleware.RetryMiddleware(config))
}

// WithCircuitBreaker adds circuit breaker middleware
func (f *SimpleFactory) WithCircuitBreaker(wormhole *Wormhole, threshold int, timeout time.Duration) *Wormhole {
	return wormhole.Use(middleware.CircuitBreakerMiddleware(threshold, timeout))
}

// WithCache adds caching middleware
func (f *SimpleFactory) WithCache(wormhole *Wormhole, ttl time.Duration) *Wormhole {
	cache := middleware.NewMemoryCache(1000)
	config := middleware.CacheConfig{
		Cache: cache,
		TTL:   ttl,
	}
	return wormhole.Use(middleware.CacheMiddleware(config))
}

// WithTimeout adds timeout middleware
func (f *SimpleFactory) WithTimeout(wormhole *Wormhole, timeout time.Duration) *Wormhole {
	return wormhole.Use(middleware.TimeoutMiddleware(timeout))
}

// WithMetrics adds metrics tracking middleware
func (f *SimpleFactory) WithMetrics(wormhole *Wormhole) (*Wormhole, *middleware.Metrics) {
	metrics := middleware.NewMetrics()
	return wormhole.Use(middleware.MetricsMiddleware(metrics)), metrics
}

// WithLogging adds basic logging middleware
func (f *SimpleFactory) WithLogging(wormhole *Wormhole, logger types.Logger) *Wormhole {
	return wormhole.Use(middleware.LoggingMiddleware(logger))
}

// WithDetailedLogging adds detailed logging middleware with configuration
func (f *SimpleFactory) WithDetailedLogging(wormhole *Wormhole, logger types.Logger) *Wormhole {
	config := middleware.DefaultLoggingConfig(logger)
	return wormhole.Use(middleware.DetailedLoggingMiddleware(config))
}

// WithDebugLogging adds debug logging middleware
func (f *SimpleFactory) WithDebugLogging(wormhole *Wormhole, logger types.Logger) *Wormhole {
	return wormhole.Use(middleware.DebugLoggingMiddleware(logger))
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

// QuickGroq creates a Groq client with minimal configuration
func QuickGroq(apiKey ...string) *Wormhole {
	return Quick.Groq(apiKey...)
}

// QuickMistral creates a Mistral client with minimal configuration
func QuickMistral(apiKey ...string) *Wormhole {
	return Quick.Mistral(apiKey...)
}

// QuickOllama creates an Ollama client with minimal configuration
func QuickOllama(baseURL ...string) *Wormhole {
	return Quick.Ollama(baseURL...)
}

// QuickLMStudio creates an LMStudio client with minimal configuration
func QuickLMStudio(baseURL ...string) *Wormhole {
	return Quick.LMStudio(baseURL...)
}
