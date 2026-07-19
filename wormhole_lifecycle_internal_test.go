package wormhole

import (
	"context"
	"sync"
	"testing"
)

func TestRequestAdmissionIsClosedBeforeShutdownWaits(t *testing.T) {
	client := New(WithDiscovery(false))

	const workers = 32
	start := make(chan struct{})
	var workersWG sync.WaitGroup
	workersWG.Add(workers)
	for range workers {
		go func() {
			defer workersWG.Done()
			<-start
			for client.trackRequest() {
				client.untrackRequest()
			}
		}()
	}

	close(start)
	if err := client.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	workersWG.Wait()

	if client.trackRequest() {
		client.untrackRequest()
		t.Fatal("request admitted after shutdown")
	}
}

func TestSweepIdempotencyCache(t *testing.T) {
	client := New(WithDiscovery(false))
	client.idempotencyMu.Lock()
	if client.idempotencyCache == nil {
		client.idempotencyCache = make(map[string]*idempotencyEntry)
	}
	client.idempotencyCache["expired-key"] = &idempotencyEntry{}
	client.idempotencyMu.Unlock()

	client.sweepIdempotencyCache()

	client.idempotencyMu.Lock()
	_, exists := client.idempotencyCache["expired-key"]
	client.idempotencyMu.Unlock()
	if exists {
		t.Fatal("expired-key was not swept from cache")
	}
}
