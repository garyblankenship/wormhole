package providers

import (
	"net/http"
	"strings"

	"github.com/garyblankenship/wormhole/v2/types"
)

// BearerAuthStrategy implements Bearer token authentication
type BearerAuthStrategy struct{}

// Apply adds Bearer token authentication to the request
func (s *BearerAuthStrategy) Apply(req *http.Request, config types.ProviderConfig) error {
	if config.APIKey == "" {
		return types.NewWormholeError(types.ErrorCodeAuth, "API key is required for Bearer authentication", false)
	}
	req.Header.Set(types.HeaderAuthorization, "Bearer "+config.APIKey)
	return nil
}

// Name returns the name of the authentication strategy
func (s *BearerAuthStrategy) Name() string {
	return "bearer"
}

// ExtractKey returns the bearer token carried by the request, or "".
func (s *BearerAuthStrategy) ExtractKey(req *http.Request) string {
	if req == nil {
		return ""
	}
	token, ok := strings.CutPrefix(req.Header.Get(types.HeaderAuthorization), "Bearer ")
	if !ok {
		return ""
	}
	return token
}

// HeaderAuthStrategy implements header-based API key authentication
type HeaderAuthStrategy struct {
	HeaderName string
}

// NewHeaderAuthStrategy creates a new HeaderAuthStrategy
func NewHeaderAuthStrategy(headerName string) *HeaderAuthStrategy {
	return &HeaderAuthStrategy{HeaderName: headerName}
}

// Apply adds header-based API key authentication to the request
func (s *HeaderAuthStrategy) Apply(req *http.Request, config types.ProviderConfig) error {
	if config.APIKey == "" {
		return types.NewWormholeError(types.ErrorCodeAuth, "API key is required for header authentication", false)
	}
	req.Header.Set(s.HeaderName, config.APIKey)
	return nil
}

// Name returns the name of the authentication strategy
func (s *HeaderAuthStrategy) Name() string {
	return "header"
}

// ExtractKey returns the value of the configured header, or "".
func (s *HeaderAuthStrategy) ExtractKey(req *http.Request) string {
	if req == nil {
		return ""
	}
	return req.Header.Get(s.HeaderName)
}

// QueryParamAuthStrategy implements query parameter-based API key authentication
type QueryParamAuthStrategy struct {
	ParamName string
}

// NewQueryParamAuthStrategy creates a new QueryParamAuthStrategy
func NewQueryParamAuthStrategy(paramName string) *QueryParamAuthStrategy {
	return &QueryParamAuthStrategy{ParamName: paramName}
}

// Apply adds query parameter-based API key authentication to the request
func (s *QueryParamAuthStrategy) Apply(req *http.Request, config types.ProviderConfig) error {
	if config.APIKey == "" {
		return types.NewWormholeError(types.ErrorCodeAuth, "API key is required for query parameter authentication", false)
	}

	// Get existing query parameters
	q := req.URL.Query()
	q.Set(s.ParamName, config.APIKey)
	req.URL.RawQuery = q.Encode()

	return nil
}

// Name returns the name of the authentication strategy
func (s *QueryParamAuthStrategy) Name() string {
	return "query_param"
}

// ExtractKey returns the value of the configured query parameter, or "".
func (s *QueryParamAuthStrategy) ExtractKey(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.Query().Get(s.ParamName)
}

// NoAuthStrategy implements no authentication (for local providers like Ollama)
type NoAuthStrategy struct{}

// Apply does nothing for no authentication
func (s *NoAuthStrategy) Apply(req *http.Request, config types.ProviderConfig) error {
	// No authentication required
	return nil
}

// Name returns the name of the authentication strategy
func (s *NoAuthStrategy) Name() string {
	return "none"
}

// ExtractKey returns "" — no authentication is applied.
func (s *NoAuthStrategy) ExtractKey(req *http.Request) string {
	return ""
}

// CompositeAuthStrategy implements multiple authentication strategies
type CompositeAuthStrategy struct {
	strategies []AuthStrategy
}

// NewCompositeAuthStrategy creates a new CompositeAuthStrategy
func NewCompositeAuthStrategy(strategies ...AuthStrategy) *CompositeAuthStrategy {
	return &CompositeAuthStrategy{strategies: strategies}
}

// Apply applies all authentication strategies in order
func (s *CompositeAuthStrategy) Apply(req *http.Request, config types.ProviderConfig) error {
	for _, strategy := range s.strategies {
		if err := strategy.Apply(req, config); err != nil {
			return err
		}
	}
	return nil
}

// Name returns the name of the authentication strategy
func (s *CompositeAuthStrategy) Name() string {
	return "composite"
}

// ExtractKey returns the first non-empty key extracted by a wrapped strategy.
func (s *CompositeAuthStrategy) ExtractKey(req *http.Request) string {
	for _, strategy := range s.strategies {
		if key := strategy.ExtractKey(req); key != "" {
			return key
		}
	}
	return ""
}

// AuthStrategyFactory creates authentication strategies based on provider configuration
type AuthStrategyFactory struct{}

// CreateAuthStrategy creates an appropriate authentication strategy for a provider
func (f *AuthStrategyFactory) CreateAuthStrategy(providerName string, config types.ProviderConfig) AuthStrategy {
	switch providerName {
	case "anthropic":
		// Anthropic uses x-api-key header and anthropic-version header
		return NewCompositeAuthStrategy(
			NewHeaderAuthStrategy("x-api-key"),
			&StaticHeaderAuthStrategy{HeaderName: "anthropic-version", HeaderValue: "2023-06-01"},
		)
	case "gemini":
		// Gemini uses API key in query parameter
		return NewQueryParamAuthStrategy("key")
	case "ollama":
		// Ollama typically has no authentication
		return &NoAuthStrategy{}
	default:
		// Default to Bearer token for OpenAI and other providers
		return &BearerAuthStrategy{}
	}
}

// StaticHeaderAuthStrategy adds a static header value
type StaticHeaderAuthStrategy struct {
	HeaderName  string
	HeaderValue string
}

// Apply adds a static header to the request
func (s *StaticHeaderAuthStrategy) Apply(req *http.Request, config types.ProviderConfig) error {
	req.Header.Set(s.HeaderName, s.HeaderValue)
	return nil
}

// Name returns the name of the authentication strategy
func (s *StaticHeaderAuthStrategy) Name() string {
	return "static_header"
}

// ExtractKey returns "" — a static header carries no rotatable API key.
func (s *StaticHeaderAuthStrategy) ExtractKey(req *http.Request) string {
	return ""
}
