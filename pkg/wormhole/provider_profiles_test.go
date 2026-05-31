package wormhole

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestProviderProfilesExposeKnownProviders(t *testing.T) {
	profiles := KnownProviderProfiles()
	if len(profiles) == 0 {
		t.Fatal("expected provider profiles")
	}
	profile, ok := ProviderProfileByName("groq")
	if !ok {
		t.Fatal("expected groq profile")
	}
	if profile.DefaultBaseURL != "https://api.groq.com/openai/v1" {
		t.Fatalf("groq base URL = %q", profile.DefaultBaseURL)
	}
	if profile.Kind != providerKindOpenAICompatible {
		t.Fatalf("groq kind = %q", profile.Kind)
	}
}

func TestProfiledOpenAICompatibleUsesProfileBaseURL(t *testing.T) {
	client := New(WithGroq("test-key"), WithDiscovery(false))
	cfg, ok := client.config.Providers["groq"]
	if !ok {
		t.Fatal("groq provider was not configured")
	}
	if cfg.BaseURL != "https://api.groq.com/openai/v1" {
		t.Fatalf("groq base URL = %q", cfg.BaseURL)
	}
}

func TestProfiledOpenAICompatibleAllowsConfigOverride(t *testing.T) {
	client := New(WithGroq("test-key", types.ProviderConfig{BaseURL: "http://localhost:9999/v1"}), WithDiscovery(false))
	if got := client.config.Providers["groq"].BaseURL; got != "http://localhost:9999/v1" {
		t.Fatalf("base URL override = %q", got)
	}
}

func TestWithProviderFromEnvUsesProfileEnvNames(t *testing.T) {
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
}
