package wormhole

import (
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestProviderProfilesExposeKnownProviders(t *testing.T) {
	t.Parallel()
	names := KnownProviderNames()
	if len(names) == 0 {
		t.Fatal("expected known provider names")
	}
	profiles := KnownProviderProfiles()
	if len(profiles) == 0 {
		t.Fatal("expected provider profiles")
	}

	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "deepseek", baseURL: "https://api.deepseek.com"},
		{name: "groq", baseURL: "https://api.groq.com/openai/v1"},
		{name: "synthetic", baseURL: "https://api.synthetic.new/v1"},
		{name: "zai", baseURL: "https://api.z.ai/api/coding/paas/v4"},
	}
	for _, tt := range tests {
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

	openai, ok := ProviderProfileByName("openai")
	if !ok {
		t.Fatal("expected openai profile")
	}
	if openai.RequestPolicy.MaxTokensParam != "max_tokens" {
		t.Fatalf("openai max token param = %q", openai.RequestPolicy.MaxTokensParam)
	}
	if len(openai.RequestPolicy.MaxTokensParamRules) != 1 {
		t.Fatalf("openai max token rules = %#v", openai.RequestPolicy.MaxTokensParamRules)
	}

	openrouter, ok := ProviderProfileByName("openrouter")
	if !ok {
		t.Fatal("expected openrouter profile")
	}
	if openrouter.ImagePath != "/images" {
		t.Fatalf("openrouter image path = %q", openrouter.ImagePath)
	}
}

func TestProviderProfileGettersDetachNestedState(t *testing.T) {
	t.Parallel()

	openAI, ok := ProviderProfileByName("openai")
	if !ok {
		t.Fatal("openai profile missing")
	}
	openAI.APIKeyEnv[0] = "MUTATED"
	openAI.RequestPolicy.MaxTokensParamRules[0].Param = "mutated"

	deepseek, ok := ProviderProfileByName("deepseek")
	if !ok {
		t.Fatal("deepseek profile missing")
	}
	deepseek.DefaultProviderOptions["thinking"].(map[string]any)["type"] = "mutated"

	again, _ := ProviderProfileByName("openai")
	if again.APIKeyEnv[0] == "MUTATED" || again.RequestPolicy.MaxTokensParamRules[0].Param == "mutated" {
		t.Fatal("provider profile getter exposed nested registry state")
	}
	againDeepseek, _ := ProviderProfileByName("deepseek")
	if againDeepseek.DefaultProviderOptions["thinking"].(map[string]any)["type"] == "mutated" {
		t.Fatal("provider profile getter exposed nested default options")
	}
}

func TestProfiledOpenAICompatibleUsesProfileBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "deepseek", baseURL: "https://api.deepseek.com"},
		{name: "groq", baseURL: "https://api.groq.com/openai/v1"},
		{name: "synthetic", baseURL: "https://api.synthetic.new/v1"},
		{name: "zai", baseURL: "https://api.z.ai/api/coding/paas/v4"},
	}
	for _, tt := range tests {
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

func TestDeepSeekProfileDisablesThinkingByDefault(t *testing.T) {
	t.Parallel()

	client := New(WithProfiledOpenAICompatible("deepseek", types.ProviderConfig{APIKey: "test-key"}), WithDiscovery(false))
	cfg, ok := client.config.Providers["deepseek"]
	if !ok {
		t.Fatal("deepseek provider was not configured")
	}
	thinking, ok := cfg.DefaultProviderOptions["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("deepseek thinking option = %#v, want object", cfg.DefaultProviderOptions["thinking"])
	}
	if thinking["type"] != "disabled" {
		t.Fatalf("deepseek thinking.type = %#v, want disabled", thinking["type"])
	}
}

func TestProfileDefaultProviderOptionsPreserveConfigOverride(t *testing.T) {
	t.Parallel()

	client := New(WithProfiledOpenAICompatible("deepseek", types.NewProviderConfig("test-key").
		WithDefaultProviderOptions(map[string]any{
			"thinking": map[string]any{"type": "enabled"},
			"user_id":  "request-owner",
		})), WithDiscovery(false))
	cfg := client.config.Providers["deepseek"]
	thinking := cfg.DefaultProviderOptions["thinking"].(map[string]any)
	if thinking["type"] != "enabled" {
		t.Fatalf("thinking.type = %#v, want explicit enabled", thinking["type"])
	}
	if cfg.DefaultProviderOptions["user_id"] != "request-owner" {
		t.Fatalf("user_id = %#v, want explicit request-owner", cfg.DefaultProviderOptions["user_id"])
	}
}

func TestProfiledOpenAICompatibleAllowsConfigOverride(t *testing.T) {
	t.Parallel()
	client := New(WithGroq("test-key", types.ProviderConfig{BaseURL: "http://localhost:9999/v1"}), WithDiscovery(false))
	if got := client.config.Providers["groq"].BaseURL; got != "http://localhost:9999/v1" {
		t.Fatalf("base URL override = %q", got)
	}
}

func TestProfiledOpenAICompatibleUsesProfileImagePath(t *testing.T) {
	t.Parallel()

	client := New(WithProfiledOpenAICompatible("openrouter", types.ProviderConfig{APIKey: "test-key"}), WithDiscovery(false))
	cfg, ok := client.config.Providers["openrouter"]
	if !ok {
		t.Fatal("openrouter provider was not configured")
	}
	if cfg.ImagePath != "/images" {
		t.Fatalf("openrouter image path = %q", cfg.ImagePath)
	}

	override := New(WithProfiledOpenAICompatible("openrouter", types.ProviderConfig{
		APIKey:    "test-key",
		ImagePath: "/custom/images",
	}), WithDiscovery(false))
	if got := override.config.Providers["openrouter"].ImagePath; got != "/custom/images" {
		t.Fatalf("openrouter image path override = %q", got)
	}
}

func TestWithProviderFromEnvOpenRouterAppliesProfile(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	client := New(WithProviderFromEnv("openrouter"), WithDiscovery(false))
	cfg, ok := client.config.Providers["openrouter"]
	if !ok {
		t.Fatal("openrouter provider was not configured")
	}
	if cfg.ImagePath != "/images" {
		t.Fatalf("openrouter image path = %q, want /images (WithProviderFromEnv did not apply the provider profile)", cfg.ImagePath)
	}
}

func TestWithOpenAICompatibleOpenRouterAppliesProfile(t *testing.T) {
	t.Parallel()

	client := New(WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
		APIKey: "test-key",
	}), WithDiscovery(false))
	cfg, ok := client.config.Providers["openrouter"]
	if !ok {
		t.Fatal("openrouter provider was not configured")
	}
	if cfg.ImagePath != "/images" {
		t.Fatalf("openrouter image path = %q, want /images", cfg.ImagePath)
	}
}

func TestWithProviderFromEnvDefaultBranchAppliesProfile(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "test-key")

	client := New(WithProviderFromEnv("deepseek"), WithDiscovery(false))
	cfg, ok := client.config.Providers["deepseek"]
	if !ok {
		t.Fatal("deepseek provider was not configured")
	}
	thinking, ok := cfg.DefaultProviderOptions["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("deepseek thinking.type = %#v, want disabled (WithProviderFromEnv default branch did not apply the provider profile)", cfg.DefaultProviderOptions["thinking"])
	}
}

func TestProviderProfileRequestPolicyFlowsIntoConfig(t *testing.T) {
	t.Parallel()
	client := New(WithOpenAI("test-key"), WithDiscovery(false))
	cfg := client.config.Providers["openai"]
	if cfg.RequestPolicy.MaxTokensParam != "max_tokens" {
		t.Fatalf("max token param = %q", cfg.RequestPolicy.MaxTokensParam)
	}
	if len(cfg.RequestPolicy.MaxTokensParamRules) != 1 {
		t.Fatalf("max token rules = %#v", cfg.RequestPolicy.MaxTokensParamRules)
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

func TestApplyProviderProfileResponsesTransport(t *testing.T) {
	t.Parallel()

	// Profile defaults propagate into an empty config.
	cfg := types.ProviderConfig{}
	applyProviderProfile(ProviderProfile{UseResponsesAPI: true, ResponsesPath: "/responses"}, &cfg)
	if !cfg.UseResponsesAPI {
		t.Fatalf("UseResponsesAPI = false, want true from profile")
	}
	if cfg.ResponsesPath != "/responses" {
		t.Fatalf("ResponsesPath = %q, want %q from profile", cfg.ResponsesPath, "/responses")
	}

	// Caller-set ResponsesPath is not clobbered by the profile default.
	override := types.ProviderConfig{ResponsesPath: "/custom/responses"}
	applyProviderProfile(ProviderProfile{ResponsesPath: "/responses"}, &override)
	if override.ResponsesPath != "/custom/responses" {
		t.Fatalf("ResponsesPath override = %q, want %q", override.ResponsesPath, "/custom/responses")
	}

	// A profile that does not enable the Responses transport leaves config off.
	off := types.ProviderConfig{}
	applyProviderProfile(ProviderProfile{ResponsesPath: "/responses"}, &off)
	if off.UseResponsesAPI {
		t.Fatalf("UseResponsesAPI = true, want false when profile does not enable it")
	}
}
