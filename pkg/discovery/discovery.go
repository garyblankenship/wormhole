package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// DiscoveryService fetches and caches models from providers
type DiscoveryService struct {
	cache    *ModelCache
	fetchers map[string]ModelFetcher
	config   DiscoveryConfig
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewDiscoveryService creates a new model discovery service
func NewDiscoveryService(config DiscoveryConfig, fetchers ...ModelFetcher) *DiscoveryService {
	// Use defaults if not set
	if config.CacheTTL == 0 {
		config = DefaultConfig()
	}

	s := &DiscoveryService{
		cache:    NewModelCache(config),
		fetchers: make(map[string]ModelFetcher),
		config:   config,
		stopCh:   make(chan struct{}),
	}

	// Register fetchers
	for _, fetcher := range fetchers {
		s.fetchers[fetcher.Name()] = fetcher
	}

	return s
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
			go s.refreshProvider(provider)
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
	errors := make([]error, 0, len(providers))
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to refresh some providers: %v", errors)
	}

	return nil
}

// StartBackgroundRefresh starts a goroutine that periodically refreshes models
func (s *DiscoveryService) StartBackgroundRefresh(ctx context.Context) {
	if s.config.OfflineMode || s.config.RefreshInterval == 0 {
		return // Background refresh disabled
	}

	ticker := time.NewTicker(s.config.RefreshInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				// Refresh all providers (errors logged but not returned in background)
				_ = s.RefreshModels(ctx)
			case <-s.stopCh:
				ticker.Stop()
				return
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop halts background refresh
func (s *DiscoveryService) Stop() {
	close(s.stopCh)
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
	// Use background context with timeout
	refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ignore errors in background refresh (best effort)
	_, _ = s.fetchModels(refreshCtx, provider)
}

// ClearCache clears all cached models
func (s *DiscoveryService) ClearCache() {
	s.cache.Clear()
}
