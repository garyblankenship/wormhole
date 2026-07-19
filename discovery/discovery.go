package discovery

import (
	"context"
	"sort"
	"sync"

	"github.com/garyblankenship/wormhole/v2/types"
)

// DiscoveryService fetches and caches models from providers
type DiscoveryService struct {
	cache     *ModelCache
	fetchers  map[string]ModelFetcher
	config    DiscoveryConfig
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	muStop    sync.RWMutex // serializes wg.Add (refreshProvider) against wg.Wait (Stop)
	stopped   bool         // set under muStop write lock before Stop's wg.Wait

	refreshMu       sync.Mutex          // protects refreshInFlight
	refreshInFlight map[string]struct{} // providers with a background refresh already running (dedup)
}

// NewDiscoveryService creates a new model discovery service
func NewDiscoveryService(config DiscoveryConfig, fetchers ...ModelFetcher) *DiscoveryService {
	config = NormalizeConfig(config)

	ctx, cancel := context.WithCancel(context.Background())
	s := &DiscoveryService{
		cache:           NewModelCache(config),
		fetchers:        make(map[string]ModelFetcher),
		config:          config,
		ctx:             ctx,
		cancel:          cancel,
		stopCh:          make(chan struct{}),
		refreshInFlight: make(map[string]struct{}),
	}

	// Register fetchers
	for _, fetcher := range fetchers {
		s.fetchers[fetcher.Name()] = fetcher
	}

	return s
}

// NormalizeConfig applies discovery defaults while preserving explicit toggles.
func NormalizeConfig(config DiscoveryConfig) DiscoveryConfig {
	defaults := DefaultConfig()
	if config == (DiscoveryConfig{}) {
		return defaults
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = defaults.CacheTTL
	}
	if config.FileCachePath == "" {
		config.FileCachePath = defaults.FileCachePath
	}
	if config.FileCacheTTL == 0 {
		config.FileCacheTTL = defaults.FileCacheTTL
	}
	if config.RefreshInterval == 0 && !config.DisableBackgroundRefresh {
		config.RefreshInterval = defaults.RefreshInterval
	}
	if config.DisableFileCache {
		config.EnableFileCache = false
	}
	return config
}

// MergeConfig overlays a partial config on top of defaults.
func MergeConfig(base, override DiscoveryConfig) DiscoveryConfig {
	if base == (DiscoveryConfig{}) {
		base = DefaultConfig()
	}
	if override.CacheTTL != 0 {
		base.CacheTTL = override.CacheTTL
	}
	if override.FileCachePath != "" {
		base.FileCachePath = override.FileCachePath
	}
	if override.FileCacheTTL != 0 {
		base.FileCacheTTL = override.FileCacheTTL
	}
	if override.RefreshInterval != 0 {
		base.RefreshInterval = override.RefreshInterval
	}
	if override.DisableBackgroundRefresh {
		base.RefreshInterval = 0
		base.DisableBackgroundRefresh = true
	}
	if override.EnableFileCache {
		base.EnableFileCache = true
	}
	if override.DisableFileCache {
		base.EnableFileCache = false
		base.DisableFileCache = true
	}
	if override.OfflineMode {
		base.OfflineMode = true
	}
	return base
}

// Providers returns the provider names with registered model fetchers.
func (s *DiscoveryService) Providers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	providers := make([]string, 0, len(s.fetchers))
	for provider := range s.fetchers {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	return providers
}

// RegisterFetcher adds a model fetcher for a provider
func (s *DiscoveryService) RegisterFetcher(fetcher ModelFetcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetchers[fetcher.Name()] = fetcher
}

// ModelsResult wraps discovered models together with a freshness indicator.
type ModelsResult struct {
	Models []*types.ModelInfo
	Stale  bool // true when Models came from stale cache or hardcoded fallback, not a live fetch
}

// accountCacheKey returns the on-disk cache key for provider, incorporating
// a credential discriminator when fetcher implements AccountScoped — this
// prevents cache collisions between different API keys/accounts registered
// under the same provider name.
func accountCacheKey(provider string, fetcher ModelFetcher) string {
	if as, ok := fetcher.(AccountScoped); ok {
		if disc := as.AccountDiscriminator(); disc != "" {
			return provider + "__" + disc
		}
	}
	return provider
}

// cacheKey looks up the registered fetcher for provider and returns its
// account-scoped cache key (see accountCacheKey). Falls back to provider
// alone if no fetcher is registered.
func (s *DiscoveryService) cacheKey(provider string) string {
	s.mu.RLock()
	fetcher, ok := s.fetchers[provider]
	s.mu.RUnlock()
	if !ok {
		return provider
	}
	return accountCacheKey(provider, fetcher)
}
