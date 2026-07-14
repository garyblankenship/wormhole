package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCheckerStatusAndProviders(t *testing.T) {
	t.Parallel()
	checker := NewHealthChecker(time.Hour)
	checker.SetCheckFunction(func(ctx context.Context, provider string) error {
		if provider == "bad" {
			return errors.New("bad provider")
		}
		return nil
	})

	checker.Start([]string{"good", "bad"})
	t.Cleanup(checker.Stop)

	assert.True(t, checker.IsHealthy("good"))
	assert.True(t, checker.IsHealthy("unknown"))

	checker.checkAll([]string{"good", "bad"})
	checker.checkAll([]string{"bad"})
	checker.checkAll([]string{"bad"})

	goodStatus := checker.GetStatus("good")
	assert.True(t, goodStatus.Healthy)
	assert.NoError(t, goodStatus.LastError)
	assert.Greater(t, goodStatus.LastCheck.UnixNano(), int64(0))

	badStatus := checker.GetStatus("bad")
	assert.False(t, badStatus.Healthy)
	assert.GreaterOrEqual(t, badStatus.ConsecutiveFails, 3)
	require.Error(t, badStatus.LastError)

	assert.Equal(t, []string{"good"}, checker.GetHealthyProviders([]string{"good", "bad"}))
}

func TestHealthCheckMiddleware(t *testing.T) {
	t.Parallel()
	checker := NewHealthChecker(time.Hour)
	provider := "openai"

	handler := HealthCheckMiddleware(checker, provider)(func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	})

	resp, err := handler(context.Background(), "request")
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.True(t, checker.IsHealthy(provider))

	failing := HealthCheckMiddleware(checker, provider)(func(ctx context.Context, req any) (any, error) {
		return nil, errors.New("request failed")
	})
	for i := 0; i < 3; i++ {
		_, err = failing(context.Background(), "request")
		require.Error(t, err)
	}
	assert.False(t, checker.IsHealthy(provider))

	_, err = handler(context.Background(), "request")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request failed")

	checker.statuses[provider].LastError = nil
	_, err = handler(context.Background(), "request")
	require.ErrorIs(t, err, ErrProviderUnhealthy)
}
