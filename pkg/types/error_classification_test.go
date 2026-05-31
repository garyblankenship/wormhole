package types

import (
	"errors"
	"net/http"
	"testing"
)

func TestClassifyErrorUsesTypedErrors(t *testing.T) {
	tests := []struct {
		err  error
		want ErrorClass
	}{
		{ErrRateLimited, ErrorClassRateLimit},
		{ErrQuotaExceeded, ErrorClassQuota},
		{ErrInvalidAPIKey, ErrorClassAuth},
		{ErrInvalidRequest, ErrorClassConfig},
		{ErrTimeout, ErrorClassTimeout},
		{ErrNetworkError, ErrorClassNetwork},
	}
	for _, tt := range tests {
		if got := ClassifyError(tt.err); got != tt.want {
			t.Fatalf("ClassifyError(%v) = %s, want %s", tt.err, got, tt.want)
		}
	}
}

func TestClassifyErrorUsesStatusAndTextFallbacks(t *testing.T) {
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
