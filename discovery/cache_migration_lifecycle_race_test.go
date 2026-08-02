package discovery

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeMigrationSource(t *testing.T, cache *ModelCache, provider, source string) {
	t.Helper()
	entry := &CacheEntry{
		SchemaVersion: cacheSchemaVersion,
		Models:        testModels(provider),
		Timestamp:     time.Now(),
		Provider:      provider,
	}
	var (
		path  string
		value any
	)
	switch source {
	case "legacy":
		path = cache.getLegacyProviderFilePath(provider)
		value = entry
	case "monolithic":
		path = cache.filePath
		value = FileCache{Entries: map[string]*CacheEntry{provider: entry}}
	default:
		t.Fatalf("unknown migration source %q", source)
	}
	data, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func TestModelCacheAdmittedMigrationFinishesBeforeClear(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"legacy", "monolithic"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()

			cache := newFileBackedCache(t)
			writeMigrationSource(t, cache, "test", source)
			writeStarted, allowWrite := pauseShardWrites(cache)
			getDone := make(chan struct{})
			go func() {
				models, fresh := cache.Get("test")
				assert.True(t, fresh)
				assert.Len(t, models, 1)
				close(getDone)
			}()
			<-writeStarted
			<-getDone

			clearDone := make(chan struct{})
			go func() {
				cache.Clear()
				close(clearDone)
			}()
			requireLifecycleWriterQueued(t, cache, clearDone)

			close(allowWrite)
			<-clearDone
			assert.Zero(t, cache.Size())
			assert.NoFileExists(t, cache.filePath)
			assert.NoFileExists(t, cache.getLegacyProviderFilePath("test"))
			assert.NoFileExists(t, cache.getProviderFilePath("test"))
		})
	}
}

func TestModelCacheAdmittedMigrationFinishesBeforeClose(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"legacy", "monolithic"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()

			cache := newFileBackedCache(t)
			writeMigrationSource(t, cache, "test", source)
			writeStarted, allowWrite := pauseShardWrites(cache)
			getDone := make(chan struct{})
			go func() {
				models, fresh := cache.Get("test")
				assert.True(t, fresh)
				assert.Len(t, models, 1)
				close(getDone)
			}()
			<-writeStarted
			<-getDone

			closeDone := make(chan struct{})
			var closeErr error
			go func() {
				closeErr = cache.Close()
				close(closeDone)
			}()
			requireLifecycleWriterQueued(t, cache, closeDone)

			close(allowWrite)
			<-closeDone
			require.NoError(t, closeErr)
			shardPath := cache.getProviderFilePath("test")
			assert.FileExists(t, shardPath)

			require.NoError(t, os.Remove(shardPath))
			cache.memoryMu.Lock()
			delete(cache.memory, "test")
			cache.memoryMu.Unlock()
			models, fresh := cache.Get("test")
			require.True(t, fresh)
			require.Len(t, models, 1)
			cache.wg.Wait()
			assert.NoFileExists(t, shardPath)
		})
	}
}
