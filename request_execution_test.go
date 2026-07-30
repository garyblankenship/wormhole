package wormhole

import (
	"sync"
	"testing"
	"time"
)

func TestRemoveIdempotencyEntryOnlyRemovesCurrentEntry(t *testing.T) {
	t.Parallel()

	stale := &idempotencyEntry{}
	current := &idempotencyEntry{}
	client := &Wormhole{
		idempotencyCache: map[string]*idempotencyEntry{
			"request": current,
		},
	}

	client.removeIdempotencyEntry("request", stale)
	if got := client.idempotencyCache["request"]; got != current {
		t.Fatalf("cache entry after stale removal = %p, want current entry %p", got, current)
	}

	client.removeIdempotencyEntry("request", current)
	if _, ok := client.idempotencyCache["request"]; ok {
		t.Fatal("current cache entry was not removed")
	}
}

func TestIdempotencyInFlightEntryDoesNotExpire(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	ttl := time.Second
	client := &Wormhole{idempotencyCache: make(map[string]*idempotencyEntry)}

	first, created := client.loadOrCreateIdempotencyEntry("same", now)
	if !created {
		t.Fatal("first entry was not created")
	}
	if first.state != idempotencyInFlight {
		t.Fatalf("first entry state = %v, want in-flight", first.state)
	}

	duplicate, created := client.loadOrCreateIdempotencyEntry("same", now.Add(2*ttl))
	if created {
		t.Fatal("in-flight entry was replaced after the response TTL elapsed")
	}
	if duplicate != first {
		t.Fatalf("duplicate entry = %p, want original in-flight entry %p", duplicate, first)
	}
}

func TestIdempotencyTTLStartsAtSuccessfulCompletion(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0)
	ttl := time.Second
	client := &Wormhole{idempotencyCache: make(map[string]*idempotencyEntry)}

	first, created := client.loadOrCreateIdempotencyEntry("same", now)
	if !created {
		t.Fatal("first entry was not created")
	}
	completedAt := now.Add(2 * ttl)
	client.completeIdempotencyEntry(first, completedAt, ttl)

	duplicate, created := client.loadOrCreateIdempotencyEntry("same", completedAt.Add(ttl-time.Nanosecond))
	if created || duplicate != first {
		t.Fatal("completed response was not retained for its full post-completion TTL")
	}

	replacement, created := client.loadOrCreateIdempotencyEntry("same", completedAt.Add(ttl))
	if !created || replacement == first {
		t.Fatal("completed response was not replaced after its post-completion TTL")
	}
}

func TestIdempotencySweeperSkipsInFlight(t *testing.T) {
	t.Parallel()

	client := &Wormhole{idempotencyCache: make(map[string]*idempotencyEntry)}
	inFlight, created := client.loadOrCreateIdempotencyEntry("in-flight", time.Now())
	if !created {
		t.Fatal("in-flight entry was not created")
	}

	completed, created := client.loadOrCreateIdempotencyEntry("completed", time.Now())
	if !created {
		t.Fatal("completed entry was not created")
	}
	client.completeIdempotencyEntry(completed, time.Now().Add(-time.Second), time.Nanosecond)

	client.sweepIdempotencyCache()

	client.idempotencyMu.Lock()
	defer client.idempotencyMu.Unlock()
	if client.idempotencyCache["in-flight"] != inFlight {
		t.Fatal("sweeper removed in-flight entry")
	}
	if _, ok := client.idempotencyCache["completed"]; ok {
		t.Fatal("sweeper retained expired completed entry")
	}
}

func TestIdempotencyCompletionAndSweepRace(t *testing.T) {
	t.Parallel()

	const iterations = 100
	client := &Wormhole{idempotencyCache: make(map[string]*idempotencyEntry)}

	for i := 0; i < iterations; i++ {
		key := "same"
		entry, created := client.loadOrCreateIdempotencyEntry(key, time.Now())
		if !created {
			t.Fatal("entry was not created")
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			client.completeIdempotencyEntry(entry, time.Now(), time.Hour)
		}()
		go func() {
			defer wg.Done()
			client.sweepIdempotencyCache()
		}()
		wg.Wait()

		client.idempotencyMu.Lock()
		current := client.idempotencyCache[key]
		client.idempotencyMu.Unlock()
		if current != entry {
			t.Fatalf("iteration %d: completed entry was swept", i)
		}
		client.removeIdempotencyEntry(key, entry)
	}
}

func TestIdempotencyClearDuringFlightDoesNotStrandWaiters(t *testing.T) {
	t.Parallel()

	client := &Wormhole{idempotencyCache: make(map[string]*idempotencyEntry)}
	old, created := client.loadOrCreateIdempotencyEntry("same", time.Now())
	if !created {
		t.Fatal("old entry was not created")
	}

	waiterDone := make(chan struct{})
	go func() {
		<-old.ready
		close(waiterDone)
	}()

	client.ClearIdempotencyCache()
	current, created := client.loadOrCreateIdempotencyEntry("same", time.Now())
	if !created || current == old {
		t.Fatal("clear did not permit a new idempotency entry")
	}

	old.value = "old response"
	client.completeIdempotencyEntry(old, time.Now(), time.Hour)
	close(old.ready)

	select {
	case <-waiterDone:
	case <-time.After(time.Second):
		t.Fatal("waiter on cleared in-flight entry was stranded")
	}

	client.idempotencyMu.Lock()
	defer client.idempotencyMu.Unlock()
	if client.idempotencyCache["same"] != current {
		t.Fatal("cleared owner overwrote the newer current entry")
	}
}
