package middleware

import (
	"context"
	"math"
	"runtime"
	"testing"
	"time"
)

func TestLoadBalancerHealthCheckRestartWaitsForStop(t *testing.T) {
	lb := NewLoadBalancer(RoundRobin)
	lb.healthInterval = time.Nanosecond
	lb.AddProvider("provider", func(context.Context, any) (any, error) { return nil, nil }, 1)

	entered := make(chan struct{})
	release := make(chan struct{})
	lb.StartHealthChecks(func(Handler) error {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		return nil
	})
	<-entered

	stopped := make(chan struct{})
	go func() {
		lb.StopHealthChecks()
		close(stopped)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		lb.mu.RLock()
		stopping := lb.stopHealthCheck == nil
		lb.mu.RUnlock()
		if stopping {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("health-check stop did not begin")
		}
		runtime.Gosched()
	}

	restarted := make(chan struct{})
	go func() {
		lb.StartHealthChecks(func(Handler) error { return nil })
		close(restarted)
	}()

	select {
	case <-restarted:
		t.Fatal("health checks restarted before the previous loop stopped")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("health-check stop did not complete")
	}
	select {
	case <-restarted:
	case <-time.After(time.Second):
		t.Fatal("health checks did not restart after stop completed")
	}
	lb.StopHealthChecks()
}

func TestLoadBalancerRoundRobinCounterWrap(t *testing.T) {
	providers := []*ProviderHandler{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	lb := NewLoadBalancer(RoundRobin)

	if got := lb.selectRoundRobin(providers).Name; got != "b" {
		t.Fatalf("first round-robin selection = %q, want b", got)
	}

	lb.currentIndex.Store(math.MaxUint64)
	if got := lb.selectRoundRobin(providers).Name; got != "a" {
		t.Fatalf("wrapped round-robin selection = %q, want a", got)
	}

	lb.currentIndex.Store(math.MaxUint64)
	weighted := []*ProviderHandler{{Name: "a", Weight: 1}, {Name: "b", Weight: 2}}
	if got := lb.selectWeightedRoundRobin(weighted).Name; got != "a" {
		t.Fatalf("wrapped weighted selection = %q, want a", got)
	}
}
