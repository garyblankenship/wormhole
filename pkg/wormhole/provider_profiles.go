package wormhole

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
)

//go:embed provider_profiles.json
var providerProfilesJSON []byte

// ProviderProfile describes the stable configuration shape for a known provider.
type ProviderProfile struct {
	Name           string   `json:"name"`
	DisplayName    string   `json:"display_name,omitempty"`
	Kind           string   `json:"kind"`
	DefaultBaseURL string   `json:"default_base_url,omitempty"`
	APIKeyEnv      []string `json:"api_key_env,omitempty"`
	BaseURLEnv     string   `json:"base_url_env,omitempty"`
	Discovery      string   `json:"discovery,omitempty"`
	AutoEnv        bool     `json:"auto_env,omitempty"`
	Local          bool     `json:"local,omitempty"`
}

const (
	providerKindOpenAICompatible = "openai-compatible"

	discoveryOpenAI           = "openai"
	discoveryAnthropic        = "anthropic"
	discoveryGemini           = "gemini"
	discoveryOllama           = "ollama"
	discoveryOpenRouter       = "openrouter"
	discoveryOpenAICompatible = "openai-compatible"
)

var providerProfiles struct {
	once sync.Once
	list []ProviderProfile
	by   map[string]ProviderProfile
	err  error
}

// KnownProviderNames returns the names of all built-in provider profiles, sorted alphabetically.
// This is the authoritative list of provider names for prefix routing and validation.
func KnownProviderNames() []string {
	profiles, _ := loadProviderProfiles()
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	return names
}

// KnownProviderProfiles returns all built-in provider profiles sorted by name.
func KnownProviderProfiles() []ProviderProfile {
	profiles, _ := loadProviderProfiles()
	out := make([]ProviderProfile, len(profiles))
	copy(out, profiles)
	return out
}

// ProviderProfileByName returns the built-in profile for a provider.
func ProviderProfileByName(name string) (ProviderProfile, bool) {
	_, byName := loadProviderProfiles()
	profile, ok := byName[name]
	return profile, ok
}

func loadProviderProfiles() ([]ProviderProfile, map[string]ProviderProfile) {
	providerProfiles.once.Do(func() {
		var profiles []ProviderProfile
		if err := json.Unmarshal(providerProfilesJSON, &profiles); err != nil {
			providerProfiles.err = fmt.Errorf("load provider profiles: %w", err)
			return
		}
		sort.Slice(profiles, func(i, j int) bool {
			return profiles[i].Name < profiles[j].Name
		})
		byName := make(map[string]ProviderProfile, len(profiles))
		for _, profile := range profiles {
			if profile.Name == "" {
				providerProfiles.err = fmt.Errorf("load provider profiles: empty provider name")
				return
			}
			byName[profile.Name] = profile
		}
		providerProfiles.list = profiles
		providerProfiles.by = byName
	})
	if providerProfiles.err != nil {
		return nil, nil
	}
	return providerProfiles.list, providerProfiles.by
}

func providerProfile(name string) (ProviderProfile, bool) {
	return ProviderProfileByName(name)
}

func configuredBaseURL(profile ProviderProfile) string {
	if profile.BaseURLEnv != "" {
		if value := os.Getenv(profile.BaseURLEnv); value != "" {
			return value
		}
	}
	return profile.DefaultBaseURL
}

func configuredAPIKey(profile ProviderProfile) string {
	for _, env := range profile.APIKeyEnv {
		if value := os.Getenv(env); value != "" {
			return value
		}
	}
	return ""
}

func envProviderProfiles() []ProviderProfile {
	profiles, _ := loadProviderProfiles()
	out := make([]ProviderProfile, 0, len(profiles))
	for _, profile := range profiles {
		if profile.AutoEnv {
			out = append(out, profile)
		}
	}
	return out
}
