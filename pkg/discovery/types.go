package discovery

import (
	"context"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ModelFetcher defines the interface for fetching models from a provider
type ModelFetcher interface {
	// Name returns the provider name (e.g., "openai", "anthropic")
	Name() string

	// FetchModels retrieves all available models from the provider
	FetchModels(ctx context.Context) ([]*types.ModelInfo, error)
}

// DiscoveryConfig configures the model discovery service
type DiscoveryConfig struct {
	// CacheTTL is how long to cache models in memory (default: 24h)
	CacheTTL time.Duration

	// FileCachePath is where to persist cached models (default: ~/.wormhole/models.json)
	FileCachePath string

	// EnableFileCache enables persistent file-based caching (default: true)
	EnableFileCache bool

	// RefreshInterval is how often to refresh models in background (default: 12h)
	RefreshInterval time.Duration

	// OfflineMode disables all network fetching, uses cache/fallback only (default: false)
	OfflineMode bool

	// FileCacheTTL is how long file cache is valid (default: 7 days)
	FileCacheTTL time.Duration
}

// DefaultConfig returns the default discovery configuration
func DefaultConfig() DiscoveryConfig {
	return DiscoveryConfig{
		CacheTTL:        24 * time.Hour,
		FileCachePath:   "~/.wormhole/models.json",
		EnableFileCache: true,
		RefreshInterval: 12 * time.Hour,
		OfflineMode:     false,
		FileCacheTTL:    7 * 24 * time.Hour, // 7 days
	}
}

// CacheEntry represents a cached set of models with timestamp
type CacheEntry struct {
	Models    []*types.ModelInfo `json:"models"`
	Timestamp time.Time          `json:"timestamp"`
	Provider  string             `json:"provider"`
}

// FileCache represents the structure of the persisted cache file
type FileCache struct {
	Version string                 `json:"version"`
	Updated time.Time              `json:"updated"`
	Entries map[string]*CacheEntry `json:"entries"` // provider -> CacheEntry
}
