package main

import (
	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

// Explicit local endpoints opt in without enabling unconfigured local services.
func explicitLocalProviderOptions(getenv func(string) string) []wormhole.Option {
	var opts []wormhole.Option
	for _, profile := range wormhole.KnownProviderProfiles() {
		if !profile.Local || profile.Kind != "openai-compatible" || profile.BaseURLEnv == "" {
			continue
		}
		if baseURL := getenv(profile.BaseURLEnv); baseURL != "" {
			opts = append(opts, wormhole.WithProfiledOpenAICompatible(profile.Name, types.ProviderConfig{BaseURL: baseURL, NoAuth: true, DynamicModels: true}))
		}
	}
	return opts
}
