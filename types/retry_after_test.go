package types

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRetryAfterHeader_Seconds(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Set("Retry-After", "30")
	assert.Equal(t, 30*time.Second, ParseRetryAfterHeader(h, time.Unix(0, 0)))
}

func TestParseRetryAfterHeader_HTTPDate(t *testing.T) {
	t.Parallel()
	now := time.Date(2025, 10, 21, 7, 0, 0, 0, time.UTC)
	h := http.Header{}
	h.Set("Retry-After", "Wed, 21 Oct 2025 07:28:00 GMT")
	got := ParseRetryAfterHeader(h, now)
	assert.Positive(t, got)
	assert.Equal(t, 28*time.Minute, got)
}

func TestParseRetryAfterHeader_ResetRequestsCompactDuration(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Set("x-ratelimit-reset-requests", "1m26.4s")
	got := ParseRetryAfterHeader(h, time.Unix(0, 0))
	want, err := time.ParseDuration("1m26.4s")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestParseRetryAfterHeader_NoMetadata(t *testing.T) {
	t.Parallel()
	assert.Equal(t, time.Duration(0), ParseRetryAfterHeader(http.Header{}, time.Unix(0, 0)))
}

func TestWithMethods_PreserveRetryAfter(t *testing.T) {
	t.Parallel()
	base := NewWormholeError(ErrorCodeRateLimit, "rate limited", true).
		WithRetryAfter(7 * time.Second)
	require.Equal(t, 7*time.Second, base.RetryAfter)

	cases := map[string]*WormholeError{
		"WithStatusCode": base.WithStatusCode(503),
		"WithDetails":    base.WithDetails("extra"),
		"WithProvider":   base.WithProvider("openai"),
		"WithModel":      base.WithModel("gpt-5.2"),
		"WithCause":      base.WithCause(assert.AnError),
	}
	for name, got := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, 7*time.Second, got.RetryAfter)
		})
	}
}

func TestGetRetryAfter_PrefersExplicitOverCodeDefault(t *testing.T) {
	t.Parallel()
	// Rate-limit code defaults to 30s; an explicit hint must win.
	withHint := NewWormholeError(ErrorCodeRateLimit, "rate limited", true).
		WithRetryAfter(42 * time.Second)
	assert.Equal(t, 42*time.Second, GetRetryAfter(withHint))

	// Without a hint, the existing code-based default is preserved.
	noHint := NewWormholeError(ErrorCodeRateLimit, "rate limited", true)
	assert.Equal(t, 30*time.Second, GetRetryAfter(noHint))
}
