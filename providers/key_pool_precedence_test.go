package providers

import (
	"slices"
	"testing"
	"time"
)

func TestKeyPoolInitialAPIKey(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, initial, first, next string
	}{
		{name: "rotation-only", first: "key-A", next: "key-B"},
		{name: "explicit-outside", initial: "key-explicit", first: "key-explicit", next: "key-A"},
		{name: "explicit-inside", initial: "key-B", first: "key-B", next: "key-C"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			keys := []string{"key-A", "key-B", "key-C"}
			original := slices.Clone(keys)
			pool := newKeyPool(keys, time.Minute, tc.initial)
			now := time.Unix(0, 0)
			if got := pool.currentKey(now); got != tc.first {
				t.Fatalf("initial key = %q, want %q", got, tc.first)
			}
			if got := pool.rotateAfterRateLimit(tc.first, time.Minute, now); got != tc.next {
				t.Fatalf("rotated key = %q, want %q", got, tc.next)
			}
			if got := pool.currentKey(now); got != tc.next {
				t.Fatalf("current key = %q, want %q", got, tc.next)
			}
			if !slices.Equal(keys, original) {
				t.Fatalf("caller keys changed: %q", keys)
			}
			// The pool must retain its own keys after caller configuration changes.
			for i := range keys {
				keys[i] = "caller-replaced"
			}
			if got := pool.currentKey(now); got != tc.next {
				t.Fatalf("pool aliases caller keys: %q", got)
			}
		})
	}
}
