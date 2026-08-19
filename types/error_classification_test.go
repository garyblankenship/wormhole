package types

import (
	"errors"
	"net/http"
	"testing"
)

func TestClassifyErrorUsesTypedErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want ErrorClass
	}{
		{name: "nil", err: nil, want: ErrorClassUnknown},
		{name: "rate limit", err: ErrRateLimited, want: ErrorClassRateLimit},
		{name: "quota", err: ErrQuotaExceeded, want: ErrorClassQuota},
		{name: "auth", err: ErrInvalidAPIKey, want: ErrorClassAuth},
		{name: "request", err: ErrInvalidRequest, want: ErrorClassConfig},
		{name: "model", err: ErrInvalidModel, want: ErrorClassConfig},
		{name: "validation", err: ErrValidation, want: ErrorClassConfig},
		{name: "timeout", err: ErrTimeout, want: ErrorClassTimeout},
		{name: "network", err: ErrNetworkError, want: ErrorClassNetwork},
		{name: "retryable provider", err: ErrProviderUnavailable, want: ErrorClassTransient},
		{name: "non-retryable provider", err: ErrProviderConstraintError, want: ErrorClassConfig},
		{name: "unknown code", err: NewWormholeError(ErrorCodeUnknown, "unknown", false), want: ErrorClassUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyError(tt.err); got != tt.want {
				t.Fatalf("ClassifyError(%v) = %s, want %s", tt.err, got, tt.want)
			}
		})
	}
}

func TestClassifyErrorUsesStatusAndTextFallbacks(t *testing.T) {
	t.Parallel()
	err := NewWormholeError(ErrorCodeProvider, "provider failed", true).WithStatusCode(http.StatusTooManyRequests)
	if got := ClassifyError(err); got != ErrorClassRateLimit {
		t.Fatalf("status classification = %s", got)
	}
	if got := ClassifyError(errors.New("invalid api key")); got != ErrorClassAuth {
		t.Fatalf("text classification = %s", got)
	}
	if !ErrorClassAuth.OpensProviderCircuit() || ErrorClassTransient.OpensProviderCircuit() {
		t.Fatal("unexpected circuit impact")
	}
}

func TestClassifyErrorTextFallbacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		message string
		want    ErrorClass
	}{
		{message: "too many requests", want: ErrorClassRateLimit},
		{message: "quota exceeded", want: ErrorClassQuota},
		{message: "unauthorized", want: ErrorClassAuth},
		{message: "model not configured", want: ErrorClassConfig},
		{message: "deadline exceeded", want: ErrorClassTimeout},
		{message: "connection reset", want: ErrorClassNetwork},
		{message: "unclassified failure", want: ErrorClassTransient},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyError(errors.New(tt.message)); got != tt.want {
				t.Fatalf("ClassifyError(%q) = %s, want %s", tt.message, got, tt.want)
			}
		})
	}
}

func TestClassifyStatusCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status int
		want   ErrorClass
		ok     bool
	}{
		{status: http.StatusTooManyRequests, want: ErrorClassRateLimit, ok: true},
		{status: http.StatusUnauthorized, want: ErrorClassAuth, ok: true},
		{status: http.StatusForbidden, want: ErrorClassQuota, ok: true},
		{status: http.StatusBadRequest, want: ErrorClassConfig, ok: true},
		{status: http.StatusNotFound, want: ErrorClassConfig, ok: true},
		{status: http.StatusUnprocessableEntity, want: ErrorClassConfig, ok: true},
		{status: http.StatusServiceUnavailable, want: ErrorClassTransient, ok: false},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			t.Parallel()
			got, ok := ClassifyStatusCode(tt.status)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("ClassifyStatusCode(%d) = (%s, %t), want (%s, %t)", tt.status, got, ok, tt.want, tt.ok)
			}
		})
	}
}
