package discovery

import (
	"context"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// fetchModels fetches models from a provider and updates cache. staleOK is
// reserved for ordinary reads and background refreshes; manual refreshes must
// report the live failure even when valid cached data remains available.
func (s *DiscoveryService) fetchModels(ctx context.Context, provider string, staleOK bool) ([]*types.ModelInfo, error) {
	s.mu.RLock()
	fetcher, ok := s.fetchers[provider]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no fetcher registered for provider: %s", provider)
	}

	// Fetch with timeout and service-lifecycle cancellation. Stop must cancel
	// live calls even when their caller context remains active.
	serviceCtx, stopServiceCancel := s.contextWithServiceCancellation(ctx)
	defer stopServiceCancel()
	fetchCtx, cancel := context.WithTimeout(serviceCtx, 30*time.Second)
	defer cancel()

	models, err := fetcher.FetchModels(fetchCtx)
	key := accountCacheKey(provider, fetcher)
	if err != nil {
		// Ordinary reads may keep serving the last known catalog when the live
		// endpoint is unavailable. Strict manual refreshes report the failure.
		if staleOK {
			if cached := s.cache.GetStale(key); len(cached) > 0 {
				return cached, nil
			}
			if fallback, _ := s.cache.Get(key); len(fallback) > 0 {
				return fallback, nil
			}
		}
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}

	// Update cache
	s.cache.Set(key, models)

	return models, nil
}

func (s *DiscoveryService) contextWithServiceCancellation(ctx context.Context) (context.Context, context.CancelFunc) {
	combined, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	return combined, func() {
		stop()
		cancel()
	}
}

// refreshProvider refreshes a single provider in background. If a refresh
// for this provider is already in flight, this call is a no-op (dedup
// prevents a thundering herd of redundant refreshes against a down provider).
func (s *DiscoveryService) refreshProvider(provider string) {
	s.muStop.RLock()
	defer s.muStop.RUnlock()
	if s.stopped {
		return
	}

	s.refreshMu.Lock()
	if _, inFlight := s.refreshInFlight[provider]; inFlight {
		s.refreshMu.Unlock()
		return
	}
	s.refreshInFlight[provider] = struct{}{}
	s.refreshMu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.refreshMu.Lock()
			delete(s.refreshInFlight, provider)
			s.refreshMu.Unlock()
		}()
		// Use service context with timeout for proper cancellation
		refreshCtx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
		defer cancel()

		// Ignore errors in background refresh (best effort)
		_, _ = s.fetchModels(refreshCtx, provider, true)
	}()
}

// ClearCache clears all cached models
func (s *DiscoveryService) ClearCache() {
	s.cache.Clear()
}
