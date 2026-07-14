package wormhole

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAdaptiveLimiter_ZeroAdjustmentIntervalDoesNotPanic(t *testing.T) {
	t.Parallel()
	al := NewAdaptiveLimiter(AdaptiveConfig{})
	defer al.Stop()
	assert.Equal(t, DefaultAdaptiveConfig().AdjustmentInterval, al.config.AdjustmentInterval,
		"zero AdjustmentInterval must default to avoid time.NewTicker(0) panic")
}
