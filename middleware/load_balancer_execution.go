package middleware

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

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

	return resp, wrapIfNotWormholeError("load_balancer", err)
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
	lb.lifecycleMu.Lock()
	defer lb.lifecycleMu.Unlock()

	lb.mu.Lock()
	if lb.stopHealthCheck != nil {
		lb.healthCheckFunc = checkFunc
		lb.mu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	lb.healthCheckFunc = checkFunc
	lb.stopHealthCheck = stopCh
	lb.healthWG.Add(1)
	lb.mu.Unlock()

	go lb.runHealthChecks(stopCh)
}

// StopHealthChecks stops background health checking
func (lb *LoadBalancer) StopHealthChecks() {
	lb.lifecycleMu.Lock()
	defer lb.lifecycleMu.Unlock()

	lb.mu.Lock()
	stopCh := lb.stopHealthCheck
	lb.stopHealthCheck = nil
	lb.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
		lb.healthWG.Wait()
	}
}

func (lb *LoadBalancer) runHealthChecks(stopCh <-chan struct{}) {
	defer lb.healthWG.Done()
	ticker := time.NewTicker(lb.healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
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

			if !atomic.CompareAndSwapInt32(&p.healthChecking, 0, 1) {
				p.recordHealthCheck(errHealthCheckTimeout)
				return
			}

			errCh := make(chan error, 1)
			go func() {
				defer atomic.StoreInt32(&p.healthChecking, 0)
				errCh <- checkFunc(p.Handler)
			}()

			var err error
			select {
			case err = <-errCh:
			case <-time.After(healthCheckTimeout):
				err = errHealthCheckTimeout
			}

			p.recordHealthCheck(err)
		}(provider)
	}

	wg.Wait()
}
