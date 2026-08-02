package discovery

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelCacheStartCleanupNonpositiveIntervalDisablesCleanup(t *testing.T) {
	t.Parallel()

	for _, interval := range []time.Duration{0, -time.Second} {
		t.Run(interval.String(), func(t *testing.T) {
			t.Parallel()

			cache := NewModelCache(DiscoveryConfig{
				CacheTTL:        time.Hour,
				FileCacheTTL:    time.Hour,
				FileCachePath:   filepath.Join(t.TempDir(), "models.json"),
				EnableFileCache: false,
			})
			cache.StartCleanup(interval)
			require.NoError(t, cache.Close())
			require.NoError(t, cache.Close())
		})
	}
}

func TestModelCacheCleanupAdmissionCannotOvertakeClose(t *testing.T) {
	t.Parallel()

	cache := NewModelCache(DiscoveryConfig{
		CacheTTL:        time.Hour,
		FileCacheTTL:    time.Hour,
		FileCachePath:   filepath.Join(t.TempDir(), "models.json"),
		EnableFileCache: false,
	})
	cache.muClosed.RLock()

	closeDone := make(chan struct{})
	var closeErr error
	go func() {
		closeErr = cache.Close()
		close(closeDone)
	}()
	requireLifecycleWriterQueued(t, cache, closeDone)

	admitted := make(chan bool, 1)
	go func() {
		admitted <- cache.admitCleanup(time.Hour)
	}()

	cache.muClosed.RUnlock()
	<-closeDone
	require.NoError(t, closeErr)
	require.False(t, <-admitted)
}

func TestModelCacheStartCleanupAfterCloseDoesNotAdmitWorker(t *testing.T) {
	t.Parallel()

	cache := NewModelCache(DiscoveryConfig{
		CacheTTL:        time.Hour,
		FileCacheTTL:    time.Hour,
		FileCachePath:   filepath.Join(t.TempDir(), "models.json"),
		EnableFileCache: false,
	})
	require.NoError(t, cache.Close())

	assert.False(t, cache.admitCleanup(time.Hour))
	cache.StartCleanup(time.Hour)
	cache.wg.Wait()
}
