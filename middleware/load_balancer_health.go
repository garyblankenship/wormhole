package middleware

import (
	"context"
	"sync/atomic"
	"time"
)

func (p *ProviderHandler) recordHealthCheck(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err == nil {
		p.consecutiveSuccesses++
		p.consecutiveFails = 0
		if p.consecutiveSuccesses >= healthCheckHysteresis {
			p.Healthy = true
		}
	} else {
		p.consecutiveFails++
		p.consecutiveSuccesses = 0
		if p.consecutiveFails >= healthCheckHysteresis {
			p.Healthy = false
		}
	}
	p.LastHealthCheck = time.Now()
}

// GetProviderStats returns statistics for all providers
func (lb *LoadBalancer) GetProviderStats() []ProviderStats {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	stats := make([]ProviderStats, 0, len(lb.providers))

	for _, p := range lb.providers {
		p.mu.RLock()
		stats = append(stats, ProviderStats{
			Name:              p.Name,
			Healthy:           p.Healthy,
			ActiveConnections: atomic.LoadInt32(&p.ActiveConnections),
			TotalRequests:     p.TotalRequests,
			TotalErrors:       p.TotalErrors,
			AverageLatency:    p.AverageLatency,
			LastHealthCheck:   p.LastHealthCheck,
		})
		p.mu.RUnlock()
	}

	return stats
}

// ProviderStats contains provider statistics
type ProviderStats struct {
	Name              string
	Healthy           bool
	ActiveConnections int32
	TotalRequests     int64
	TotalErrors       int64
	AverageLatency    time.Duration
	LastHealthCheck   time.Time
}

// LoadBalancerMiddleware creates a middleware that load balances across multiple handlers
func LoadBalancerMiddleware(strategy LoadBalanceStrategy, providers map[string]Handler) Middleware {
	lb := NewLoadBalancer(strategy)

	for name, handler := range providers {
		lb.AddProvider(name, handler, 1)
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, req any) (any, error) {
			result, err := lb.Execute(ctx, req)
			return result, wrapIfNotWormholeError("load_balancer", err)
		}
	}
}
