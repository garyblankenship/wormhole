package wormhole

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"time"
)

// Close implements io.Closer interface for Wormhole.
func (p *Wormhole) Close() error {
	return p.Shutdown(context.Background())
}

// Shutdown gracefully shuts down the Wormhole client with zero-downtime support.
func (p *Wormhole) Shutdown(ctx context.Context) error {
	var shutdownErr error
	p.shutdownOnce.Do(func() {
		p.signalShutdown()

		done := make(chan struct{})
		go func() {
			p.activeRequests.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
			shutdownErr = fmt.Errorf("shutdown timeout: %w", ctx.Err())
		}

		var errs []error

		p.providersMutex.Lock()
		for name, cp := range p.providers {
			if err := cp.provider.Close(); err != nil {
				errs = append(errs, fmt.Errorf("provider %s: %w", name, err))
			}
			delete(p.providers, name)
		}
		p.providersMutex.Unlock()

		if p.discoveryService != nil {
			if err := p.discoveryService.Stop(); err != nil {
				errs = append(errs, fmt.Errorf("discovery service: %w", err))
			}
		}

		if p.adaptiveLimiter != nil {
			p.adaptiveLimiter.Stop()
		}

		if len(errs) > 0 {
			shutdownErr = fmt.Errorf("errors during shutdown cleanup: %v", errs)
		}
	})

	return shutdownErr
}

func (p *Wormhole) signalShutdown() {
	p.shuttingDown.Store(true)
	select {
	case <-p.shutdownChan:
		return
	default:
		close(p.shutdownChan)
	}
}

// IsShuttingDown returns true if the client is in shutdown process.
func (p *Wormhole) IsShuttingDown() bool {
	return p.shuttingDown.Load()
}

func (p *Wormhole) trackRequest() bool {
	if p.shuttingDown.Load() {
		return false
	}
	p.activeRequests.Add(1)
	if p.shuttingDown.Load() {
		p.activeRequests.Done()
		return false
	}
	return true
}

func (p *Wormhole) untrackRequest() {
	p.activeRequests.Done()
}

// ClearIdempotencyCache clears all cached idempotent responses.
func (p *Wormhole) ClearIdempotencyCache() {
	p.idempotencyMu.Lock()
	defer p.idempotencyMu.Unlock()
	clear(p.idempotencyCache)
}

// CleanupStaleProviders cleans up providers that haven't been used for a while.
func (p *Wormhole) CleanupStaleProviders(maxAge time.Duration, maxCount int) {
	p.providersMutex.Lock()
	defer p.providersMutex.Unlock()

	now := time.Now()
	staleKeys := []string{}
	for name, cp := range p.providers {
		refCount := atomic.LoadInt32(&cp.refCount)
		lastUsed := atomic.LoadInt64(&cp.lastUsed)
		if refCount == 0 && now.Sub(time.Unix(0, lastUsed)) > maxAge {
			staleKeys = append(staleKeys, name)
		}
	}

	for _, name := range staleKeys {
		if cp, ok := p.providers[name]; ok {
			if err := cp.provider.Close(); err != nil && p.config.Logger != nil {
				p.config.Logger.Warn("error closing stale provider", "provider", name, "error", err)
			}
			delete(p.providers, name)
			p.cacheEvictions.Add(1)
		}
	}

	if maxCount > 0 && len(p.providers) > maxCount {
		type providerInfo struct {
			name     string
			lastUsed int64
		}
		unusedProviders := make([]providerInfo, 0, len(p.providers))

		for name, cp := range p.providers {
			if atomic.LoadInt32(&cp.refCount) == 0 {
				unusedProviders = append(unusedProviders, providerInfo{
					name:     name,
					lastUsed: atomic.LoadInt64(&cp.lastUsed),
				})
			}
		}

		sort.Slice(unusedProviders, func(i, j int) bool {
			return unusedProviders[i].lastUsed < unusedProviders[j].lastUsed
		})

		neededEvictions := len(p.providers) - maxCount
		for i := 0; i < neededEvictions && i < len(unusedProviders); i++ {
			name := unusedProviders[i].name
			if cp, ok := p.providers[name]; ok {
				if err := cp.provider.Close(); err != nil && p.config.Logger != nil {
					p.config.Logger.Warn("error closing provider during LRU eviction", "provider", name, "error", err)
				}
				delete(p.providers, name)
				p.cacheEvictions.Add(1)
			}
		}

		if len(p.providers) > maxCount && p.config.Logger != nil {
			p.config.Logger.Warn("provider cache exceeds max count but all providers are in use",
				"current", len(p.providers), "max", maxCount)
		}
	}
}

// CacheMetrics holds cache performance statistics.
type CacheMetrics struct {
	Hits      int64
	Misses    int64
	Evictions int64
	Size      int
}

// GetCacheMetrics returns current cache performance statistics.
func (p *Wormhole) GetCacheMetrics() CacheMetrics {
	p.providersMutex.RLock()
	defer p.providersMutex.RUnlock()

	return CacheMetrics{
		Hits:      p.cacheHits.Load(),
		Misses:    p.cacheMisses.Load(),
		Evictions: p.cacheEvictions.Load(),
		Size:      len(p.providers),
	}
}

// EnableAdaptiveConcurrency enables adaptive concurrency control with the given configuration.
func (p *Wormhole) EnableAdaptiveConcurrency(config *EnhancedAdaptiveConfig) {
	if config == nil {
		defaultConfig := DefaultEnhancedAdaptiveConfig()
		config = &defaultConfig
	}

	if p.adaptiveLimiter != nil {
		p.adaptiveLimiter.Stop()
	}

	p.adaptiveLimiter = NewEnhancedAdaptiveLimiter(*config)
}

// GetAdaptiveLimiter returns the adaptive limiter if enabled, or nil.
func (p *Wormhole) GetAdaptiveLimiter() *EnhancedAdaptiveLimiter {
	return p.adaptiveLimiter
}

// GetAdaptiveConcurrencyStats returns statistics from the adaptive limiter if enabled.
func (p *Wormhole) GetAdaptiveConcurrencyStats() map[string]interface{} {
	if p.adaptiveLimiter == nil {
		return nil
	}
	return p.adaptiveLimiter.GetStats()
}
