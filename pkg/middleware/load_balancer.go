package middleware

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/garyblankenship/wormhole/pkg/config"
)

var (
	// ErrNoHealthyProviders is returned when no healthy providers are available
	ErrNoHealthyProviders = errors.New("no healthy providers available")
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
	Name              string
	Handler           Handler
	Weight            int
	ActiveConnections int32
	TotalRequests     int64
	TotalErrors       int64
	AverageLatency    time.Duration
	LastHealthCheck   time.Time
	Healthy           bool
	mu                sync.RWMutex
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
		return nil, ErrNoHealthyProviders
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

		connectionScore := 1.0 / (float64(p.ActiveConnections) + 1.0)

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

// Execute runs a request through the load balancer
func (lb *LoadBalancer) Execute(ctx context.Context, req any) (any, error) {
	provider, err := lb.SelectProvider(ctx)
	if err != nil {
		return nil, err
	}

	// Increment active connections
	atomic.AddInt32(&provider.ActiveConnections, 1)
	defer atomic.AddInt32(&provider.ActiveConnections, -1)

	// Track timing
	start := time.Now()

	// Execute request
	resp, err := provider.Handler(ctx, req)

	// Update metrics
	lb.updateProviderMetrics(provider, time.Since(start), err)

	return resp, err
}

func (lb *LoadBalancer) updateProviderMetrics(provider *ProviderHandler, latency time.Duration, err error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	provider.TotalRequests++

	if err != nil {
		provider.TotalErrors++
	}

	// Update average latency (exponential moving average)
	if provider.AverageLatency == 0 {
		provider.AverageLatency = latency
	} else {
		provider.AverageLatency = (provider.AverageLatency*9 + latency) / 10
	}
}

// StartHealthChecks starts background health checking
func (lb *LoadBalancer) StartHealthChecks(checkFunc func(Handler) error) {
	lb.mu.Lock()
	lb.healthCheckFunc = checkFunc
	lb.stopHealthCheck = make(chan struct{})
	lb.mu.Unlock()

	go lb.runHealthChecks()
}

// StopHealthChecks stops background health checking
func (lb *LoadBalancer) StopHealthChecks() {
	lb.mu.Lock()
	if lb.stopHealthCheck != nil {
		close(lb.stopHealthCheck)
		lb.stopHealthCheck = nil
	}
	lb.mu.Unlock()
}

func (lb *LoadBalancer) runHealthChecks() {
	ticker := time.NewTicker(lb.healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lb.stopHealthCheck:
			return
		case <-ticker.C:
			lb.performHealthChecks()
		}
	}
}

func (lb *LoadBalancer) performHealthChecks() {
	lb.mu.RLock()
	providers := make([]*ProviderHandler, len(lb.providers))
	copy(providers, lb.providers)
	checkFunc := lb.healthCheckFunc
	lb.mu.RUnlock()

	if checkFunc == nil {
		return
	}

	var wg sync.WaitGroup
	for _, provider := range providers {
		wg.Add(1)
		go func(p *ProviderHandler) {
			defer wg.Done()

			err := checkFunc(p.Handler)

			p.mu.Lock()
			p.Healthy = err == nil
			p.LastHealthCheck = time.Now()
			p.mu.Unlock()
		}(provider)
	}

	wg.Wait()
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
			return lb.Execute(ctx, req)
		}
	}
}
