package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreakerErrorPolicyOpensFastForTerminalClasses(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"rate limit", types.ErrRateLimited},
		{"quota", types.ErrQuotaExceeded},
		{"auth", types.ErrInvalidAPIKey},
		{"config", types.ErrInvalidRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker(5, time.Hour)
			_, err := cb.Execute(context.Background(), func() (any, error) {
				return nil, tt.err
			})
			require.Error(t, err)
			assert.Equal(t, StateOpen, cb.GetState())
		})
	}
}

func TestCircuitBreakerErrorPolicyKeepsTransientThreshold(t *testing.T) {
	cb := NewCircuitBreaker(5, time.Hour)
	_, err := cb.Execute(context.Background(), func() (any, error) {
		return nil, errors.New("temporary failure")
	})
	require.Error(t, err)
	assert.Equal(t, StateClosed, cb.GetState())
}
