package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test 1: rotation fires on a 429 within the retry path, using the next key.
func TestKeyRotationFiresOnRetry(t *testing.T) {
	t.Parallel()

	var attempt int64
	var mu sync.Mutex
	var seen []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&attempt, 1) - 1 // 0-based attempt index
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		if n == 0 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	maxRetries := 2
	retryDelay := 1 * time.Millisecond
	config := types.ProviderConfig{
		BaseURL:    server.URL,
		APIKeys:    []string{"key-A", "key-B"},
		MaxRetries: &maxRetries,
		RetryDelay: &retryDelay,
	}

	wrapper := NewHTTPClientWrapper("test", config, nil, &BearerAuthStrategy{}, server.Client())

	var out map[string]any
	err := wrapper.DoRequest(context.Background(), http.MethodPost, server.URL, nil, &out)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"Bearer key-A", "Bearer key-B"}, seen)
}

func TestKeyRotationFiresOnStreamRetry(t *testing.T) {
	t.Parallel()

	var attempt int64
	var mu sync.Mutex
	var seen []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&attempt, 1) - 1
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		if n == 0 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set(types.HeaderContentType, types.ContentTypeEventStream)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: ok\n\n"))
	}))
	t.Cleanup(server.Close)

	maxRetries := 2
	retryDelay := time.Millisecond
	config := types.ProviderConfig{
		BaseURL:    server.URL,
		APIKeys:    []string{"key-A", "key-B"},
		MaxRetries: &maxRetries,
		RetryDelay: &retryDelay,
	}

	wrapper := NewHTTPClientWrapper("test", config, nil, &BearerAuthStrategy{}, server.Client())

	body, err := wrapper.StreamRequest(context.Background(), http.MethodPost, server.URL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = body.Close() })

	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "data: ok\n\n", string(data))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"Bearer key-A", "Bearer key-B"}, seen)
}

// Test 2: with MaxRetries:0 the retry loop returns after attempt 0, so rotation
// NEVER fires even with a multi-key pool. The server is hit exactly once and only
// the first key is ever seen.
func TestKeyRotationDisabledWhenMaxRetriesZero(t *testing.T) {
	t.Parallel()

	var hits int64
	var mu sync.Mutex
	var seen []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)

	maxRetries := 0
	retryDelay := 1 * time.Millisecond
	config := types.ProviderConfig{
		BaseURL:    server.URL,
		APIKeys:    []string{"key-A", "key-B"},
		MaxRetries: &maxRetries,
		RetryDelay: &retryDelay,
	}

	wrapper := NewHTTPClientWrapper("test", config, nil, &BearerAuthStrategy{}, server.Client())

	var out map[string]any
	err := wrapper.DoRequest(context.Background(), http.MethodPost, server.URL, nil, &out)
	require.Error(t, err)

	assert.Equal(t, int64(1), atomic.LoadInt64(&hits))
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"Bearer key-A"}, seen)
}

func TestKeyRotationDoesNotRotateOnServerErrorRetry(t *testing.T) {
	t.Parallel()

	var attempt int64
	var mu sync.Mutex
	var seen []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&attempt, 1)
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	maxRetries := 2
	retryDelay := 1 * time.Millisecond
	config := types.ProviderConfig{
		BaseURL:    server.URL,
		APIKeys:    []string{"key-A", "key-B"},
		MaxRetries: &maxRetries,
		RetryDelay: &retryDelay,
	}

	wrapper := NewHTTPClientWrapper("test", config, nil, &BearerAuthStrategy{}, server.Client())

	var out map[string]any
	err := wrapper.DoRequest(context.Background(), http.MethodPost, server.URL, nil, &out)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"Bearer key-A", "Bearer key-A"}, seen)
}

func TestKeyPoolConcurrentRateLimitsAdvanceOnce(t *testing.T) {
	t.Parallel()

	pool := newKeyPool([]string{"key-A", "key-B", "key-C"}, time.Minute)
	var wg sync.WaitGroup
	results := make(chan string, 16)
	now := time.Now()
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- pool.rotateAfterRateLimit("key-A", 0, now)
		}()
	}
	wg.Wait()
	close(results)

	for key := range results {
		assert.Equal(t, "key-B", key)
	}
	assert.Equal(t, "key-B", pool.currentKey(now))
}

func TestKeyPoolCooldownMakesLimitedKeyAvailableAgain(t *testing.T) {
	t.Parallel()

	pool := newKeyPool([]string{"key-A", "key-B"}, time.Minute)
	now := time.Now()
	assert.Equal(t, "key-B", pool.rotateAfterRateLimit("key-A", time.Millisecond, now))
	assert.Equal(t, "key-A", pool.rotateAfterRateLimit("key-B", time.Millisecond, now.Add(2*time.Millisecond)))
}

// Regression: a provider-supplied Retry-After far larger than any sane cooldown
// (e.g. 10h) must be capped at maxKeyCooldown, not honored verbatim — otherwise a
// bogus or malicious header value benches a key for the full uncapped duration.
func TestKeyPoolCooldownCapsUnboundedRetryAfter(t *testing.T) {
	t.Parallel()

	pool := newKeyPool([]string{"key-A", "key-B"}, time.Minute)
	now := time.Now()
	pool.rotateAfterRateLimit("key-A", 10*time.Hour, now)

	pool.mu.Lock()
	limitedUntil := pool.limited[0]
	pool.mu.Unlock()

	if got := limitedUntil.Sub(now); got != maxKeyCooldown {
		t.Fatalf("cooldown = %v, want capped at %v", got, maxKeyCooldown)
	}
}

// Regression: header-auth providers (Anthropic uses x-api-key) must rotate keys on
// a 429. Before AuthStrategy.ExtractKey, the pool identified the failed key only from
// an Authorization: Bearer header, so x-api-key rotation was a silent no-op.
func TestKeyRotationFiresOnRetryWithHeaderAuth(t *testing.T) {
	t.Parallel()

	var attempt int64
	var mu sync.Mutex
	var seen []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&attempt, 1) - 1
		mu.Lock()
		seen = append(seen, r.Header.Get("x-api-key"))
		mu.Unlock()
		if n == 0 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	maxRetries := 2
	retryDelay := time.Millisecond
	config := types.ProviderConfig{
		BaseURL:    server.URL,
		APIKeys:    []string{"key-A", "key-B"},
		MaxRetries: &maxRetries,
		RetryDelay: &retryDelay,
	}

	wrapper := NewHTTPClientWrapper("test", config, nil, NewHeaderAuthStrategy("x-api-key"), server.Client())

	var out map[string]any
	err := wrapper.DoRequest(context.Background(), http.MethodPost, server.URL, nil, &out)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"key-A", "key-B"}, seen)
}

// Regression: query-param providers (Gemini uses ?key=) must rotate keys on a 429,
// and the retried request must carry the NEW key in the query string. Before the fix
// Gemini baked the key into the URL once and used NoAuthStrategy, so rotation could
// neither identify the failed key nor re-derive the URL for the next attempt.
func TestKeyRotationFiresOnRetryWithQueryParamAuth(t *testing.T) {
	t.Parallel()

	var attempt int64
	var mu sync.Mutex
	var seen []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&attempt, 1) - 1
		mu.Lock()
		seen = append(seen, r.URL.Query().Get("key"))
		mu.Unlock()
		if n == 0 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	maxRetries := 2
	retryDelay := time.Millisecond
	config := types.ProviderConfig{
		BaseURL:    server.URL,
		APIKeys:    []string{"key-A", "key-B"},
		MaxRetries: &maxRetries,
		RetryDelay: &retryDelay,
	}

	wrapper := NewHTTPClientWrapper("test", config, nil, NewQueryParamAuthStrategy("key"), server.Client())

	var out map[string]any
	err := wrapper.DoRequest(context.Background(), http.MethodPost, server.URL, nil, &out)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"key-A", "key-B"}, seen)
}
