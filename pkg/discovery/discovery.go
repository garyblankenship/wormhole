package discovery

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
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
}

// NewDiscoveryService creates a new model discovery service
func NewDiscoveryService(config DiscoveryConfig, fetchers ...ModelFetcher) *DiscoveryService {
	config = NormalizeConfig(config)

	ctx, cancel := context.WithCancel(context.Background())
	s := &DiscoveryService{
		cache:    NewModelCache(config),
		fetchers: make(map[string]ModelFetcher),
		config:   config,
		ctx:      ctx,
		cancel:   cancel,
		stopCh:   make(chan struct{}),
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

// GetModels returns models for a provider (from cache or fetch)
func (s *DiscoveryService) GetModels(ctx context.Context, provider string) ([]*types.ModelInfo, error) {
	// Check cache first
	if models, fresh := s.cache.Get(provider); len(models) > 0 {
		if !fresh {
			// Using fallback/stale cache, trigger background refresh
			s.refreshProvider(provider)
		}
		return models, nil
	}

	// Cache miss, fetch now (blocking)
	if s.config.OfflineMode {
		return nil, fmt.Errorf("no cached models for provider %s and offline mode enabled", provider)
	}

	return s.fetchModels(ctx, provider)
}

// RefreshModels manually triggers model discovery for all providers
func (s *DiscoveryService) RefreshModels(ctx context.Context) error {
	s.mu.RLock()
	providers := make([]string, 0, len(s.fetchers))
	for name := range s.fetchers {
		providers = append(providers, name)
	}
	s.mu.RUnlock()

	// Fetch all providers in parallel
	var wg sync.WaitGroup
	errCh := make(chan error, len(providers))

	for _, provider := range providers {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			if _, err := s.fetchModels(ctx, p); err != nil {
				errCh <- fmt.Errorf("%s: %w", p, err)
			}
		}(provider)
	}

	wg.Wait()
	close(errCh)

	// Collect errors (pre-allocate for expected capacity)
	errs := make([]error, 0, len(providers))
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to refresh some providers: %v", errs)
	}

	return nil
}

// StartBackgroundRefresh starts a goroutine that periodically refreshes models
func (s *DiscoveryService) StartBackgroundRefresh(ctx context.Context) {
	if s.config.OfflineMode || s.config.RefreshInterval == 0 {
		return // Background refresh disabled
	}

	s.startOnce.Do(func() {
		s.wg.Add(1)
		ticker := time.NewTicker(s.config.RefreshInterval)
		go func() {
			defer s.wg.Done()
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					// Refresh all providers (errors logged but not returned in background)
					_ = s.RefreshModels(ctx)
				case <-s.stopCh:
					return
				case <-ctx.Done():
					return
				case <-s.ctx.Done():
					return
				}
			}
		}()
	})
}

// Stop halts background refresh and cleans up resources
func (s *DiscoveryService) Stop() error {
	var err error
	s.stopOnce.Do(func() {
		s.cancel()      // Cancel context
		close(s.stopCh) // Close stop channel

		// Block new refreshProvider goroutines before waiting, so wg.Add
		// can never race wg.Wait (sync: WaitGroup misuse panic).
		s.muStop.Lock()
		s.stopped = true
		s.muStop.Unlock()

		s.wg.Wait() // Wait for all goroutines
		// Close the model cache
		err = s.cache.Close()
	})
	return err
}

// fetchModels fetches models from a provider and updates cache
func (s *DiscoveryService) fetchModels(ctx context.Context, provider string) ([]*types.ModelInfo, error) {
	s.mu.RLock()
	fetcher, ok := s.fetchers[provider]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no fetcher registered for provider: %s", provider)
	}

	// Fetch with timeout
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	models, err := fetcher.FetchModels(fetchCtx)
	if err != nil {
		// Return cached/fallback on error
		if cached, _ := s.cache.Get(provider); len(cached) > 0 {
			return cached, nil // Return stale cache
		}
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}

	// Update cache
	s.cache.Set(provider, models)

	return models, nil
}

// refreshProvider refreshes a single provider in background
func (s *DiscoveryService) refreshProvider(provider string) {
	s.muStop.RLock()
	defer s.muStop.RUnlock()
	if s.stopped {
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		// Use service context with timeout for proper cancellation
		refreshCtx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
		defer cancel()

		// Ignore errors in background refresh (best effort)
		_, _ = s.fetchModels(refreshCtx, provider)
	}()
}

// ClearCache clears all cached models
func (s *DiscoveryService) ClearCache() {
	s.cache.Clear()
}
