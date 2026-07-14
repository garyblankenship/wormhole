package discovery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()

	assert.Equal(t, 24*time.Hour, cfg.CacheTTL)
	assert.Equal(t, "~/.wormhole/models.json", cfg.FileCachePath)
	assert.True(t, cfg.EnableFileCache)
	assert.Equal(t, 12*time.Hour, cfg.RefreshInterval)
	assert.False(t, cfg.OfflineMode)
	assert.Equal(t, 7*24*time.Hour, cfg.FileCacheTTL)
}
