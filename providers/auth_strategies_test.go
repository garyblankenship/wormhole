package providers

import (
	"net/http"
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestAuthStrategiesApply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		strategy   AuthStrategy
		config     types.ProviderConfig
		assertions func(t *testing.T, req *http.Request)
		wantErr    bool
	}{
		{
			name:     "bearer sets authorization header",
			strategy: &BearerAuthStrategy{},
			config:   types.ProviderConfig{APIKey: "secret"},
			assertions: func(t *testing.T, req *http.Request) {
				t.Helper()
				if got := req.Header.Get(types.HeaderAuthorization); got != "Bearer secret" {
					t.Fatalf("Authorization header = %q, want %q", got, "Bearer secret")
				}
			},
		},
		{
			name:     "bearer rejects empty api key",
			strategy: &BearerAuthStrategy{},
			wantErr:  true,
		},
		{
			name:     "header sets configured header",
			strategy: NewHeaderAuthStrategy("x-api-key"),
			config:   types.ProviderConfig{APIKey: "secret"},
			assertions: func(t *testing.T, req *http.Request) {
				t.Helper()
				if got := req.Header.Get("x-api-key"); got != "secret" {
					t.Fatalf("x-api-key header = %q, want %q", got, "secret")
				}
			},
		},
		{
			name:     "header rejects empty api key",
			strategy: NewHeaderAuthStrategy("x-api-key"),
			wantErr:  true,
		},
		{
			name:     "query param sets configured key",
			strategy: NewQueryParamAuthStrategy("key"),
			config:   types.ProviderConfig{APIKey: "secret"},
			assertions: func(t *testing.T, req *http.Request) {
				t.Helper()
				if got := req.URL.Query().Get("key"); got != "secret" {
					t.Fatalf("query key = %q, want %q", got, "secret")
				}
			},
		},
		{
			name:     "query param rejects empty api key",
			strategy: NewQueryParamAuthStrategy("key"),
			wantErr:  true,
		},
		{
			name:     "no auth leaves request untouched",
			strategy: &NoAuthStrategy{},
			assertions: func(t *testing.T, req *http.Request) {
				t.Helper()
				if got := req.Header.Get(types.HeaderAuthorization); got != "" {
					t.Fatalf("Authorization header = %q, want empty", got)
				}
				if got := req.URL.RawQuery; got != "" {
					t.Fatalf("RawQuery = %q, want empty", got)
				}
			},
		},
		{
			name: "composite applies strategies in order",
			strategy: NewCompositeAuthStrategy(
				NewHeaderAuthStrategy("x-api-key"),
				&StaticHeaderAuthStrategy{HeaderName: "x-version", HeaderValue: "v1"},
			),
			config: types.ProviderConfig{APIKey: "secret"},
			assertions: func(t *testing.T, req *http.Request) {
				t.Helper()
				if got := req.Header.Get("x-api-key"); got != "secret" {
					t.Fatalf("x-api-key header = %q, want %q", got, "secret")
				}
				if got := req.Header.Get("x-version"); got != "v1" {
					t.Fatalf("x-version header = %q, want %q", got, "v1")
				}
			},
		},
		{
			name:     "composite returns first strategy error",
			strategy: NewCompositeAuthStrategy(NewHeaderAuthStrategy("x-api-key"), &StaticHeaderAuthStrategy{HeaderName: "x-version", HeaderValue: "v1"}),
			wantErr:  true,
			assertions: func(t *testing.T, req *http.Request) {
				t.Helper()
				if got := req.Header.Get("x-version"); got != "" {
					t.Fatalf("x-version header = %q, want empty after first error", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequest(http.MethodGet, "https://example.test/path", nil)
			if err != nil {
				t.Fatal(err)
			}

			err = tt.strategy.Apply(req, tt.config)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Apply returned nil error, want error")
				}
			} else if err != nil {
				t.Fatalf("Apply returned error: %v", err)
			}

			if tt.assertions != nil {
				tt.assertions(t, req)
			}
		})
	}
}

func TestAuthStrategyNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		strategy AuthStrategy
		want     string
	}{
		{strategy: &BearerAuthStrategy{}, want: "bearer"},
		{strategy: NewHeaderAuthStrategy("x-api-key"), want: "header"},
		{strategy: NewQueryParamAuthStrategy("key"), want: "query_param"},
		{strategy: &NoAuthStrategy{}, want: "none"},
		{strategy: NewCompositeAuthStrategy(), want: "composite"},
		{strategy: &StaticHeaderAuthStrategy{}, want: "static_header"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			if got := tt.strategy.Name(); got != tt.want {
				t.Fatalf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuthStrategyFactory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		providerName string
		wantName     string
		assertions   func(t *testing.T, req *http.Request)
	}{
		{
			providerName: "anthropic",
			wantName:     "composite",
			assertions: func(t *testing.T, req *http.Request) {
				t.Helper()
				if got := req.Header.Get("x-api-key"); got != "secret" {
					t.Fatalf("x-api-key header = %q, want %q", got, "secret")
				}
				if got := req.Header.Get("anthropic-version"); got != "2023-06-01" {
					t.Fatalf("anthropic-version header = %q, want %q", got, "2023-06-01")
				}
			},
		},
		{
			providerName: "gemini",
			wantName:     "query_param",
			assertions: func(t *testing.T, req *http.Request) {
				t.Helper()
				if got := req.URL.Query().Get("key"); got != "secret" {
					t.Fatalf("query key = %q, want %q", got, "secret")
				}
			},
		},
		{
			providerName: "ollama",
			wantName:     "none",
		},
		{
			providerName: "openai",
			wantName:     "bearer",
			assertions: func(t *testing.T, req *http.Request) {
				t.Helper()
				if got := req.Header.Get(types.HeaderAuthorization); got != "Bearer secret" {
					t.Fatalf("Authorization header = %q, want %q", got, "Bearer secret")
				}
			},
		},
	}

	factory := &AuthStrategyFactory{}
	for _, tt := range tests {
		t.Run(tt.providerName, func(t *testing.T) {
			t.Parallel()

			strategy := factory.CreateAuthStrategy(tt.providerName, types.ProviderConfig{})
			if got := strategy.Name(); got != tt.wantName {
				t.Fatalf("strategy name = %q, want %q", got, tt.wantName)
			}

			req, err := http.NewRequest(http.MethodGet, "https://example.test/path", nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := strategy.Apply(req, types.ProviderConfig{APIKey: "secret"}); err != nil {
				t.Fatalf("Apply returned error: %v", err)
			}

			if tt.assertions != nil {
				tt.assertions(t, req)
			}
		})
	}
}
