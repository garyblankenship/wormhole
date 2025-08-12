package middleware

import (
	"context"
	"errors"
	"sync"
	"time"
)

// HealthStatus represents the health status of a provider
type HealthStatus struct {
	Healthy          bool
	LastCheck        time.Time
	LastError        error
	ResponseTime     time.Duration
	ConsecutiveFails int
}

// HealthChecker monitors provider health
type HealthChecker struct {
	mu            sync.RWMutex
	statuses      map[string]*HealthStatus
	checkInterval time.Duration
	checkFunc     func(ctx context.Context, provider string) error
	stopChan      chan struct{}
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(interval time.Duration) *HealthChecker {
	return &HealthChecker{
		statuses:      make(map[string]*HealthStatus),
		checkInterval: interval,
		stopChan:      make(chan struct{}),
	}
}

// SetCheckFunction sets the function used to check provider health
func (hc *HealthChecker) SetCheckFunction(fn func(ctx context.Context, provider string) error) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.checkFunc = fn
}

// Start begins health checking
func (hc *HealthChecker) Start(providers []string) {
	// Initialize status for each provider
	hc.mu.Lock()
	for _, provider := range providers {
		if _, exists := hc.statuses[provider]; !exists {
			hc.statuses[provider] = &HealthStatus{
				Healthy:   true, // Assume healthy initially
				LastCheck: time.Now(),
			}
		}
	}
	hc.mu.Unlock()

	// Start health check goroutine
	go hc.runHealthChecks(providers)
}

// Stop stops health checking
func (hc *HealthChecker) Stop() {
	close(hc.stopChan)
}

// GetStatus returns the health status for a provider
func (hc *HealthChecker) GetStatus(provider string) *HealthStatus {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	status, exists := hc.statuses[provider]
	if !exists {
		return &HealthStatus{
			Healthy:   true, // Assume healthy if not tracked
			LastCheck: time.Now(),
		}
	}

	// Return a copy to avoid race conditions
	return &HealthStatus{
		Healthy:          status.Healthy,
		LastCheck:        status.LastCheck,
		LastError:        status.LastError,
		ResponseTime:     status.ResponseTime,
		ConsecutiveFails: status.ConsecutiveFails,
	}
}

// IsHealthy returns whether a provider is healthy
func (hc *HealthChecker) IsHealthy(provider string) bool {
	status := hc.GetStatus(provider)
	return status.Healthy
}

// GetHealthyProviders returns a list of healthy providers
func (hc *HealthChecker) GetHealthyProviders(providers []string) []string {
	healthy := make([]string, 0, len(providers))

	for _, provider := range providers {
		if hc.IsHealthy(provider) {
			healthy = append(healthy, provider)
		}
	}

	return healthy
}

func (hc *HealthChecker) runHealthChecks(providers []string) {
	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()

	// Initial check
	hc.checkAll(providers)

	for {
		select {
		case <-ticker.C:
			hc.checkAll(providers)
		case <-hc.stopChan:
			return
		}
	}
}

func (hc *HealthChecker) checkAll(providers []string) {
	if hc.checkFunc == nil {
		return
	}

	var wg sync.WaitGroup
	for _, provider := range providers {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			hc.checkProvider(p)
		}(provider)
	}
	wg.Wait()
}

func (hc *HealthChecker) checkProvider(provider string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	err := hc.checkFunc(ctx, provider)
	responseTime := time.Since(start)

	hc.mu.Lock()
	defer hc.mu.Unlock()

	status, exists := hc.statuses[provider]
	if !exists {
		status = &HealthStatus{}
		hc.statuses[provider] = status
	}

	status.LastCheck = time.Now()
	status.ResponseTime = responseTime

	if err != nil {
		status.LastError = err
		status.ConsecutiveFails++

		// Mark unhealthy after 3 consecutive failures
		if status.ConsecutiveFails >= 3 {
			status.Healthy = false
		}
	} else {
		status.Healthy = true
		status.ConsecutiveFails = 0
		status.LastError = nil
	}
}

// HealthCheckMiddleware adds health checking to requests
func HealthCheckMiddleware(checker *HealthChecker, providerName string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			// Check if provider is healthy
			if !checker.IsHealthy(providerName) {
				status := checker.GetStatus(providerName)
				if status.LastError != nil {
					return nil, status.LastError
				}
				return nil, ErrProviderUnhealthy
			}

			// Execute request and track health
			start := time.Now()
			resp, err := next(ctx, req)
			responseTime := time.Since(start)

			// Update health status based on response
			checker.mu.Lock()
			status, exists := checker.statuses[providerName]
			if !exists {
				status = &HealthStatus{}
				checker.statuses[providerName] = status
			}

			status.ResponseTime = responseTime
			status.LastCheck = time.Now()

			if err != nil {
				status.ConsecutiveFails++
				status.LastError = err
				if status.ConsecutiveFails >= 3 {
					status.Healthy = false
				}
			} else {
				status.Healthy = true
				status.ConsecutiveFails = 0
				status.LastError = nil
			}
			checker.mu.Unlock()

			return resp, err
		}
	}
}

// ErrProviderUnhealthy is returned when a provider is unhealthy
var ErrProviderUnhealthy = errors.New("provider is unhealthy")
