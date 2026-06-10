package wormhole

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestProviderProfilesExposeKnownProviders(t *testing.T) {
	t.Parallel()
	profiles := KnownProviderProfiles()
	if len(profiles) == 0 {
		t.Fatal("expected provider profiles")
	}

	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "groq", baseURL: "https://api.groq.com/openai/v1"},
		{name: "synthetic", baseURL: "https://api.synthetic.new/v1"},
		{name: "zai", baseURL: "https://api.z.ai/api/coding/paas/v4"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			profile, ok := ProviderProfileByName(tt.name)
			if !ok {
				t.Fatalf("expected %s profile", tt.name)
			}
			if profile.DefaultBaseURL != tt.baseURL {
				t.Fatalf("%s base URL = %q", tt.name, profile.DefaultBaseURL)
			}
			if profile.Kind != providerKindOpenAICompatible {
				t.Fatalf("%s kind = %q", tt.name, profile.Kind)
			}
		})
	}
}

func TestProfiledOpenAICompatibleUsesProfileBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "groq", baseURL: "https://api.groq.com/openai/v1"},
		{name: "synthetic", baseURL: "https://api.synthetic.new/v1"},
		{name: "zai", baseURL: "https://api.z.ai/api/coding/paas/v4"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := New(WithProfiledOpenAICompatible(tt.name, types.ProviderConfig{APIKey: "test-key"}), WithDiscovery(false))
			cfg, ok := client.config.Providers[tt.name]
			if !ok {
				t.Fatalf("%s provider was not configured", tt.name)
			}
			if cfg.BaseURL != tt.baseURL {
				t.Fatalf("%s base URL = %q", tt.name, cfg.BaseURL)
			}
		})
	}
}

func TestProfiledOpenAICompatibleAllowsConfigOverride(t *testing.T) {
	t.Parallel()
	client := New(WithGroq("test-key", types.ProviderConfig{BaseURL: "http://localhost:9999/v1"}), WithDiscovery(false))
	if got := client.config.Providers["groq"].BaseURL; got != "http://localhost:9999/v1" {
		t.Fatalf("base URL override = %q", got)
	}
}

func TestWithProviderFromEnvUsesProfileEnvNames(t *testing.T) {
	t.Run("gemini alternate API key", func(t *testing.T) {
		t.Setenv("GEMINI_API_KEY", "")
		t.Setenv("GOOGLE_API_KEY", "test-google-key")
		t.Setenv("GEMINI_BASE_URL", "http://gemini.test")

		client := New(WithProviderFromEnv("gemini"), WithDiscovery(false))
		cfg, ok := client.config.Providers["gemini"]
		if !ok {
			t.Fatal("gemini provider was not configured")
		}
		if cfg.APIKey != "test-google-key" || cfg.BaseURL != "http://gemini.test" {
			t.Fatalf("gemini config = %#v", cfg)
		}
	})

	t.Run("synthetic profile env", func(t *testing.T) {
		t.Setenv("SYNTHETIC_API_KEY", "test-synthetic-key")
		t.Setenv("SYNTHETIC_BASE_URL", "http://synthetic.test/v1")

		client := New(WithProviderFromEnv("synthetic"), WithDiscovery(false))
		cfg, ok := client.config.Providers["synthetic"]
		if !ok {
			t.Fatal("synthetic provider was not configured")
		}
		if cfg.APIKey != "test-synthetic-key" || cfg.BaseURL != "http://synthetic.test/v1" {
			t.Fatalf("synthetic config = %#v", cfg)
		}
	})
}
