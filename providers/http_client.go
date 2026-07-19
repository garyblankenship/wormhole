package providers

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// HTTPClient is the request-execution boundary used by providers.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// responseBodyPool pools byte slices for response bodies to reduce allocations.
// Stores *[]byte so sync.Pool.Put receives a pointer type (SA6002).
var responseBodyPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 4096)
		return &buf
	},
}

const maxProviderResponseBodyBytes = 32 << 20

func readResponseBodyLimited(r io.Reader) ([]byte, error) {
	respBody, err := readAllPooled(io.LimitReader(r, maxProviderResponseBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(respBody) > maxProviderResponseBodyBytes {
		returnResponseBuf(respBody)
		return nil, types.ErrRequestTooLarge.WithDetails(
			fmt.Sprintf("provider response body exceeded %d bytes", maxProviderResponseBodyBytes),
		)
	}
	return respBody, nil
}

// readAllPooled reads all data from r into a pooled byte slice.
// The caller MUST call returnResponseBuf after using the slice.
func readAllPooled(r io.Reader) ([]byte, error) {
	// Get initial buffer from pool
	bufPtr := responseBodyPool.Get().(*[]byte)
	buf := (*bufPtr)[:0] // reset length

	// Temporary scratch buffer for reading chunks
	scratch := make([]byte, 4096)

	for {
		n, err := r.Read(scratch)
		if n > 0 {
			// Ensure we have enough capacity
			if cap(buf)-len(buf) < n {
				// Need to grow buffer
				newCap := cap(buf) * 2
				if newCap == 0 {
					newCap = 4096
				}
				// Ensure new capacity can hold existing data + new data
				if newCap < len(buf)+n {
					newCap = len(buf) + n
				}
				newBuf := make([]byte, len(buf), newCap)
				copy(newBuf, buf)
				// Return old buffer to pool
				old := buf[:0]
				responseBodyPool.Put(&old)
				buf = newBuf
			}
			buf = append(buf, scratch[:n]...)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			// On error, return buffer to pool
			errBuf := buf[:0]
			responseBodyPool.Put(&errBuf)
			return nil, err
		}
	}
	return buf, nil
}

// returnResponseBuf returns a response buffer to the pool.
func returnResponseBuf(buf []byte) {
	buf = buf[:0]
	responseBodyPool.Put(&buf)
}

// keyPool provides thread-safe stateful selection over a set of API keys.
// maxKeyCooldown caps how long a key rotation cooldown can be, regardless of
// what a provider's Retry-After header requests. Without a cap, a bogus or
// malicious large header value (e.g. 10h) would bench a key for that long.
const maxKeyCooldown = 5 * time.Minute

type keyPool struct {
	mu       sync.Mutex
	keys     []string
	current  int
	limited  map[int]time.Time
	cooldown time.Duration
}

func newKeyPool(keys []string, cooldown time.Duration) *keyPool {
	if cooldown <= 0 {
		cooldown = time.Second
	}
	return &keyPool{
		keys:     append([]string(nil), keys...),
		limited:  make(map[int]time.Time),
		cooldown: cooldown,
	}
}

func (kp *keyPool) currentKey(now time.Time) string {
	kp.mu.Lock()
	defer kp.mu.Unlock()
	kp.expireLocked(now)
	if !kp.isLimitedLocked(kp.current, now) {
		return kp.keys[kp.current]
	}
	for offset := 1; offset < len(kp.keys); offset++ {
		next := (kp.current + offset) % len(kp.keys)
		if !kp.isLimitedLocked(next, now) {
			kp.current = next
			return kp.keys[kp.current]
		}
	}
	return kp.keys[kp.current]
}

func (kp *keyPool) rotateAfterRateLimit(failedKey string, retryAfter time.Duration, now time.Time) string {
	kp.mu.Lock()
	defer kp.mu.Unlock()
	kp.expireLocked(now)

	failedIdx := kp.indexOfLocked(failedKey)
	if failedIdx >= 0 {
		cooldown := kp.cooldown
		if retryAfter > 0 {
			cooldown = retryAfter
		}
		if cooldown > maxKeyCooldown {
			cooldown = maxKeyCooldown
		}
		kp.limited[failedIdx] = now.Add(cooldown)
	}

	// Avoid double-advancing: only move the cursor when the request that saw
	// the 429 used the currently selected key.
	if failedIdx == kp.current {
		for offset := 1; offset < len(kp.keys); offset++ {
			next := (kp.current + offset) % len(kp.keys)
			if !kp.isLimitedLocked(next, now) {
				kp.current = next
				break
			}
		}
	}

	if kp.isLimitedLocked(kp.current, now) {
		for idx := range kp.keys {
			if !kp.isLimitedLocked(idx, now) {
				kp.current = idx
				break
			}
		}
	}
	return kp.keys[kp.current]
}

func (kp *keyPool) indexOfLocked(key string) int {
	for idx, existing := range kp.keys {
		if existing == key {
			return idx
		}
	}
	return -1
}

func (kp *keyPool) expireLocked(now time.Time) {
	for idx, until := range kp.limited {
		if !until.After(now) {
			delete(kp.limited, idx)
		}
	}
}

func (kp *keyPool) isLimitedLocked(idx int, now time.Time) bool {
	until, ok := kp.limited[idx]
	return ok && until.After(now)
}
