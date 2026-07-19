package middleware

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/v2/config"
	"github.com/garyblankenship/wormhole/v2/types"
)

var (
	// ErrNoHealthyProviders is returned when no healthy providers are available
	ErrNoHealthyProviders = types.NewWormholeError(types.ErrorCodeMiddleware, "no healthy providers available", false)

	// errHealthCheckTimeout marks a provider health check that exceeded healthCheckTimeout
	errHealthCheckTimeout = errors.New("load balancer: health check timed out")
)

const (
	// healthCheckTimeout bounds a single provider health check so a hung
	// checkFunc cannot block performHealthChecks/StopHealthChecks forever.
	healthCheckTimeout = 5 * time.Second
	// healthCheckHysteresis is the number of consecutive same-direction
	// results required before flipping ProviderHandler.Healthy, preventing
	// single-sample flapping under intermittent connectivity.
	healthCheckHysteresis = 2
)

// LoadBalanceStrategy defines how to select providers
type LoadBalanceStrategy int

const (
	// RoundRobin cycles through providers in order
	RoundRobin LoadBalanceStrategy = iota
	// Random selects providers randomly
	Random
	// LeastConnections selects provider with fewest active connections
	LeastConnections
	// WeightedRoundRobin cycles through providers based on weights
	WeightedRoundRobin
	// ResponseTime selects provider with best response time
	ResponseTime
	// Adaptive dynamically adjusts based on performance
	Adaptive
)

// ProviderHandler represents a provider with its handler
type ProviderHandler struct {
	Name                 string
	Handler              Handler
	Weight               int
	ActiveConnections    int32
	TotalRequests        int64
	TotalErrors          int64
	AverageLatency       time.Duration
	LastHealthCheck      time.Time
	Healthy              bool
	consecutiveFails     int
	consecutiveSuccesses int
	healthChecking       int32
	mu                   sync.RWMutex
}

// LoadBalancer distributes requests across multiple providers
type LoadBalancer struct {
	providers       []*ProviderHandler
	strategy        LoadBalanceStrategy
	currentIndex    int32
	mu              sync.RWMutex
	healthCheckFunc func(Handler) error
	healthInterval  time.Duration
	stopHealthCheck chan struct{}
	healthWG        sync.WaitGroup
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(strategy LoadBalanceStrategy) *LoadBalancer {
	return &LoadBalancer{
		providers:      make([]*ProviderHandler, 0),
		strategy:       strategy,
		healthInterval: config.DefaultLoadBalancerHealthInterval,
	}
}

// AddProvider adds a provider to the load balancer
func (lb *LoadBalancer) AddProvider(name string, handler Handler, weight int) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	provider := &ProviderHandler{
		Name:    name,
		Handler: handler,
		Weight:  weight,
		Healthy: true,
	}

	lb.providers = append(lb.providers, provider)
}

// SelectProvider selects a provider based on the strategy
func (lb *LoadBalancer) SelectProvider(ctx context.Context) (*ProviderHandler, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	// Get healthy providers
	healthy := lb.getHealthyProviders()
	if len(healthy) == 0 {
		return nil, wrapMiddlewareError("load_balancer", "select_provider", ErrNoHealthyProviders)
	}

	switch lb.strategy {
	case RoundRobin:
		return lb.selectRoundRobin(healthy), nil
	case Random:
		return lb.selectRandom(healthy), nil
	case LeastConnections:
		return lb.selectLeastConnections(healthy), nil
	case WeightedRoundRobin:
		return lb.selectWeightedRoundRobin(healthy), nil
	case ResponseTime:
		return lb.selectResponseTime(healthy), nil
	case Adaptive:
		return lb.selectAdaptive(healthy), nil
	default:
		return lb.selectRoundRobin(healthy), nil
	}
}

func (lb *LoadBalancer) getHealthyProviders() []*ProviderHandler {
	healthy := make([]*ProviderHandler, 0, len(lb.providers))
	for _, p := range lb.providers {
		p.mu.RLock()
		if p.Healthy {
			healthy = append(healthy, p)
		}
		p.mu.RUnlock()
	}
	return healthy
}

func (lb *LoadBalancer) selectRoundRobin(providers []*ProviderHandler) *ProviderHandler {
	index := atomic.AddInt32(&lb.currentIndex, 1)
	return providers[int(index)%len(providers)]
}

func (lb *LoadBalancer) selectRandom(providers []*ProviderHandler) *ProviderHandler {
	// #nosec G404 - math/rand is acceptable for load balancing (not security-critical)
	return providers[rand.Intn(len(providers))]
}

func (lb *LoadBalancer) selectLeastConnections(providers []*ProviderHandler) *ProviderHandler {
	var selected *ProviderHandler
	minConnections := int32(^uint32(0) >> 1) // Max int32

	for _, p := range providers {
		connections := atomic.LoadInt32(&p.ActiveConnections)
		if connections < minConnections {
			minConnections = connections
			selected = p
		}
	}

	return selected
}

func (lb *LoadBalancer) selectWeightedRoundRobin(providers []*ProviderHandler) *ProviderHandler {
	totalWeight := 0
	for _, p := range providers {
		totalWeight += p.Weight
	}

	if totalWeight == 0 {
		return lb.selectRoundRobin(providers)
	}

	index := atomic.AddInt32(&lb.currentIndex, 1)
	target := int(index) % totalWeight

	current := 0
	for _, p := range providers {
		current += p.Weight
		if target < current {
			return p
		}
	}

	return providers[len(providers)-1]
}

func (lb *LoadBalancer) selectResponseTime(providers []*ProviderHandler) *ProviderHandler {
	var selected *ProviderHandler
	minLatency := time.Duration(^uint64(0) >> 1) // Max duration

	for _, p := range providers {
		p.mu.RLock()
		latency := p.AverageLatency
		p.mu.RUnlock()

		if latency < minLatency {
			minLatency = latency
			selected = p
		}
	}

	if selected == nil {
		return providers[0]
	}

	return selected
}

func (lb *LoadBalancer) selectAdaptive(providers []*ProviderHandler) *ProviderHandler {
	// Score each provider based on multiple factors
	type score struct {
		provider *ProviderHandler
		value    float64
	}

	scores := make([]score, 0, len(providers))

	for _, p := range providers {
		p.mu.RLock()

		// Calculate score based on:
		// - Active connections (lower is better)
		// - Error rate (lower is better)
		// - Response time (lower is better)

		connectionScore := 1.0 / (float64(atomic.LoadInt32(&p.ActiveConnections)) + 1.0)

		errorRate := 0.0
		if p.TotalRequests > 0 {
			errorRate = float64(p.TotalErrors) / float64(p.TotalRequests)
		}
		errorScore := 1.0 - errorRate

		// Normalize latency to 0-1 range (assuming 5s is max acceptable)
		latencyScore := 1.0 - (p.AverageLatency.Seconds() / 5.0)
		if latencyScore < 0 {
			latencyScore = 0
		}

		p.mu.RUnlock()

		// Weighted combination
		totalScore := connectionScore*0.3 + errorScore*0.4 + latencyScore*0.3

		scores = append(scores, score{provider: p, value: totalScore})
	}

	// Select provider with highest score
	var best *ProviderHandler
	maxScore := 0.0

	for _, s := range scores {
		if s.value > maxScore {
			maxScore = s.value
			best = s.provider
		}
	}

	if best == nil {
		return providers[0]
	}

	return best
}
