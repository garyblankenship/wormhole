package wormhole

import (
	"context"
	"errors"
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
	p.shutdownOnce.Do(func() {
		p.signalShutdown()
		go p.runShutdown()
	})

	select {
	case <-p.shutdownDone:
		return p.shutdownErr
	default:
	}

	select {
	case <-p.shutdownDone:
		return p.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Wormhole) runShutdown() {
	defer close(p.shutdownDone)

	p.idempotencySweepWg.Wait()
	p.activeRequests.Wait()
	p.providerAcquisitionWg.Wait()

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

	if limiter := p.adaptiveLimiter.Load(); limiter != nil {
		limiter.Stop()
	}

	for _, c := range p.closers {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		p.shutdownErr = fmt.Errorf("errors during shutdown cleanup: %w", errors.Join(errs...))
	}
}

func (p *Wormhole) signalShutdown() {
	p.requestAdmissionMu.Lock()
	defer p.requestAdmissionMu.Unlock()

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
	p.requestAdmissionMu.Lock()
	defer p.requestAdmissionMu.Unlock()

	if p.shuttingDown.Load() {
		return false
	}
	p.activeRequests.Add(1)
	return true
}

func (p *Wormhole) untrackRequest() {
	p.activeRequests.Done()
}

func (p *Wormhole) beginProviderAcquisition() bool {
	p.requestAdmissionMu.Lock()
	defer p.requestAdmissionMu.Unlock()

	if p.shuttingDown.Load() {
		return false
	}
	p.providerAcquisitionWg.Add(1)
	return true
}

func (p *Wormhole) finishProviderAcquisition() bool {
	p.requestAdmissionMu.Lock()
	defer p.requestAdmissionMu.Unlock()

	return p.shuttingDown.Load()
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
	var normalized EnhancedAdaptiveConfig
	if config == nil {
		normalized = DefaultEnhancedAdaptiveConfig()
	} else {
		normalized = normalizeEnhancedAdaptiveConfig(*config)
	}

	newLimiter := NewEnhancedAdaptiveLimiter(normalized)
	if old := p.adaptiveLimiter.Swap(newLimiter); old != nil {
		old.Stop()
	}
}

// GetAdaptiveLimiter returns the adaptive limiter if enabled, or nil.
func (p *Wormhole) GetAdaptiveLimiter() *EnhancedAdaptiveLimiter {
	return p.adaptiveLimiter.Load()
}

// GetAdaptiveConcurrencyStats returns statistics from the adaptive limiter if enabled.
func (p *Wormhole) GetAdaptiveConcurrencyStats() map[string]interface{} {
	limiter := p.adaptiveLimiter.Load()
	if limiter == nil {
		return nil
	}
	return limiter.GetStats()
}
