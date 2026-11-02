package wormhole

import (
	"time"

	"github.com/garyblankenship/wormhole/v3/providers/codex"
	"github.com/garyblankenship/wormhole/v3/types"
)

// WithCodex registers account-bound Codex text generation. authPath must be
// explicitly supplied; credentials are read at provider construction, not at
// option registration. Never pass an API key in config.
func WithCodex(authPath string, config types.ProviderConfig) Option {
	return func(c *Config) {
		if c.Providers == nil {
			c.Providers = make(map[string]types.ProviderConfig)
		}
		if c.CustomFactories == nil {
			c.CustomFactories = make(map[string]types.ProviderFactory)
		}
		if config.MaxRetries == nil {
			config = config.WithNoRetries()
		}
		c.Providers["codex"] = config
		c.CustomFactories["codex"] = func(cfg types.ProviderConfig) (types.Provider, error) {
			credential, err := codex.LoadCredential(authPath, time.Now())
			if err != nil {
				return nil, err
			}
			return codex.New(cfg, credential, nil)
		}
	}
}
