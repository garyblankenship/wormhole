package wormhole

import "testing"

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
