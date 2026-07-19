package discovery

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// GetModelsWithStatus returns models for a provider along with a Stale flag,
// distinguishing a live fetch from stale-cache/fallback data — GetModels
// cannot express this distinction since both cases return err == nil.
func (s *DiscoveryService) GetModelsWithStatus(ctx context.Context, provider string) (*ModelsResult, error) {
	// Check cache first
	if models, fresh := s.cache.Get(s.cacheKey(provider)); len(models) > 0 {
		if !fresh {
			// Using fallback/stale cache, trigger background refresh
			s.refreshProvider(provider)
		}
		return &ModelsResult{Models: models, Stale: !fresh}, nil
	}

	// Cache miss, fetch now (blocking)
	if s.config.OfflineMode {
		return nil, fmt.Errorf("no cached models for provider %s and offline mode enabled", provider)
	}

	models, err := s.fetchModels(ctx, provider, true)
	if err != nil {
		return nil, err
	}
	_, fresh := s.cache.Get(s.cacheKey(provider))
	return &ModelsResult{Models: models, Stale: !fresh}, nil
}

// GetModels returns models for a provider (from cache or fetch).
// Staleness information is discarded; use GetModelsWithStatus to distinguish
// a live fetch from stale/fallback data.
func (s *DiscoveryService) GetModels(ctx context.Context, provider string) ([]*types.ModelInfo, error) {
	result, err := s.GetModelsWithStatus(ctx, provider)
	if err != nil {
		return nil, err
	}
	return result.Models, nil
}

// RefreshModels manually triggers model discovery for all providers
func (s *DiscoveryService) RefreshModels(ctx context.Context) error {
	s.mu.RLock()
	providers := make([]string, 0, len(s.fetchers))
	for name := range s.fetchers {
		providers = append(providers, name)
	}
	s.mu.RUnlock()
	sort.Strings(providers)

	// Fetch all providers in parallel
	var wg sync.WaitGroup
	errs := make([]error, len(providers))

	for i, provider := range providers {
		wg.Add(1)
		go func(index int, p string) {
			defer wg.Done()
			if _, err := s.fetchModels(ctx, p, false); err != nil {
				errs[index] = fmt.Errorf("%s: %w", p, err)
			}
		}(i, provider)
	}

	wg.Wait()
	joined := errs[:0]
	for _, err := range errs {
		if err != nil {
			joined = append(joined, err)
		}
	}

	if len(joined) > 0 {
		return fmt.Errorf("failed to refresh providers: %w", errors.Join(joined...))
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
