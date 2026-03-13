package providers

import (
	"net/http"

	"github.com/garyblankenship/wormhole/pkg/config"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// AuthStrategy defines the interface for authentication strategies
type AuthStrategy interface {
	// Apply adds authentication to the request
	Apply(req *http.Request, config types.ProviderConfig) error
	// Name returns the name of the authentication strategy
	Name() string
}

// BaseProvider provides common functionality for all providers
// Embeds the types.BaseProvider for default method implementations
// and adds HTTP functionality for making requests
type BaseProvider struct {
	*types.BaseProvider
	Config       types.ProviderConfig
	*HTTPClientWrapper
}

// NewBaseProvider creates a new base provider with default secure TLS configuration
func NewBaseProvider(name string, providerConfig types.ProviderConfig) *BaseProvider {
	return NewBaseProviderWithAuth(name, providerConfig, nil, nil, nil)
}

// NewBaseProviderWithTLS creates a new base provider with custom TLS configuration
func NewBaseProviderWithTLS(name string, providerConfig types.ProviderConfig, tlsConfig *config.TLSConfig) *BaseProvider {
	return NewBaseProviderWithAuth(name, providerConfig, tlsConfig, nil, nil)
}

// NewBaseProviderWithAuth creates a new base provider with custom TLS and auth configuration
func NewBaseProviderWithAuth(name string, providerConfig types.ProviderConfig, tlsConfig *config.TLSConfig, authStrategy AuthStrategy, httpClient HTTPClient) *BaseProvider {
	if tlsConfig == nil {
		tlsConfig = ExtractTLSConfigFromProviderConfig(providerConfig)
	}

	if authStrategy == nil {
		authStrategy = &BearerAuthStrategy{}
	}

	bp := &BaseProvider{
		BaseProvider:      types.NewBaseProvider(name),
		Config:            providerConfig,
		HTTPClientWrapper: NewHTTPClientWrapper(name, providerConfig, tlsConfig, authStrategy, httpClient),
	}

	return bp
}

// NewInsecureBaseProvider creates a new base provider with insecure TLS configuration
func NewInsecureBaseProvider(name string, providerConfig types.ProviderConfig, skipVerify bool) *BaseProvider {
	insecureTLS := config.InsecureTLSConfig()
	if skipVerify {
		insecureTLS = insecureTLS.WithInsecureSkipVerify(true)
	}

	return NewBaseProviderWithAuth(name, providerConfig, &insecureTLS, nil, nil)
}

// GetBaseURL returns the base URL for the provider
func (p *BaseProvider) GetBaseURL() string {
	if p.Config.BaseURL != "" {
		return p.Config.BaseURL
	}
	// Default URLs will be set by specific providers
	return ""
}

// Error proxy methods
func (p *BaseProvider) NotImplementedError(method string) error {
	return types.NotImplementedError(p.Name(), method)
}

func (p *BaseProvider) ValidationError(message string, details ...string) error {
	return types.NewProviderValidationError(p.Name(), message, details...)
}

func (p *BaseProvider) ValidationErrorf(format string, args ...any) error {
	return types.ValidationErrorf(p.Name(), format, args...)
}

func (p *BaseProvider) ProviderError(message string, details ...string) error {
	return types.ProviderError(p.Name(), message, details...)
}

func (p *BaseProvider) ProviderErrorf(format string, args ...any) error {
	return types.ProviderErrorf(p.Name(), format, args...)
}

func (p *BaseProvider) RequestError(message string, cause error) error {
	return types.RequestError(p.Name(), message, cause)
}

func (p *BaseProvider) ModelError(message string, details ...string) error {
	return types.ModelError(p.Name(), message, details...)
}

func (p *BaseProvider) ModelErrorf(format string, args ...any) error {
	return types.ModelErrorf(p.Name(), format, args...)
}

func (p *BaseProvider) AuthError(message string, details ...string) error {
	return types.AuthError(p.Name(), message, details...)
}

func (p *BaseProvider) AuthErrorf(format string, args ...any) error {
	return types.AuthErrorf(p.Name(), format, args...)
}

func (p *BaseProvider) WrapError(code types.ErrorCode, message string, cause error) error {
	return types.WrapProviderError(p.Name(), code, message, cause)
}

func (p *BaseProvider) Close() error {
    return p.HTTPClientWrapper.Close()
}
