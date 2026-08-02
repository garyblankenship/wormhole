package discovery

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func pauseShardWrites(cache *ModelCache) (<-chan struct{}, chan struct{}) {
	writeStarted := make(chan struct{})
	allowWrite := make(chan struct{})
	cache.writeShard = func(path string, data []byte) error {
		close(writeStarted)
		<-allowWrite
		return writeShardAtomic(path, data)
	}
	return writeStarted, allowWrite
}

func requireLifecycleReaderActive(t *testing.T, cache *ModelCache, done <-chan struct{}) {
	t.Helper()
	for {
		select {
		case <-done:
			t.Fatal("cache operation returned before lifecycle read ownership was observed")
		default:
		}
		if !cache.muClosed.TryLock() {
			return
		}
		cache.muClosed.Unlock()
		runtime.Gosched()
	}
}

func requireLifecycleWriterQueued(t *testing.T, cache *ModelCache, done <-chan struct{}) {
	t.Helper()
	for {
		select {
		case <-done:
			t.Fatal("lifecycle operation returned while admitted cache work was active")
		default:
		}
		if !cache.muClosed.TryRLock() {
			return
		}
		cache.muClosed.RUnlock()
		runtime.Gosched()
	}
}

func TestModelCacheClearWaitsForAdmittedSave(t *testing.T) {
	t.Parallel()

	cache := newFileBackedCache(t)
	writeStarted, allowWrite := pauseShardWrites(cache)
	saveDone := make(chan struct{})
	go func() {
		cache.Set("test", testModels("test"))
		close(saveDone)
	}()
	<-writeStarted

	clearStarted := make(chan struct{})
	clearDone := make(chan struct{})
	go func() {
		close(clearStarted)
		cache.Clear()
		close(clearDone)
	}()
	<-clearStarted
	requireLifecycleWriterQueued(t, cache, clearDone)

	close(allowWrite)
	<-saveDone
	<-clearDone
	assert.Zero(t, cache.Size())
	assert.NoFileExists(t, cache.getProviderFilePath("test"))
}

func TestModelCacheCloseWaitsForAdmittedSave(t *testing.T) {
	t.Parallel()

	cache := newFileBackedCache(t)
	writeStarted, allowWrite := pauseShardWrites(cache)
	saveDone := make(chan struct{})
	go func() {
		cache.Set("test", testModels("test"))
		close(saveDone)
	}()
	<-writeStarted

	closeStarted := make(chan struct{})
	closeDone := make(chan struct{})
	var closeErr error
	go func() {
		close(closeStarted)
		closeErr = cache.Close()
		close(closeDone)
	}()
	<-closeStarted
	requireLifecycleWriterQueued(t, cache, closeDone)

	close(allowWrite)
	<-saveDone
	<-closeDone
	require.NoError(t, closeErr)
	assert.FileExists(t, cache.getProviderFilePath("test"))

	before, err := os.ReadFile(cache.getProviderFilePath("test"))
	require.NoError(t, err)
	cache.Set("test", testModels("after-close"))
	after, err := os.ReadFile(cache.getProviderFilePath("test"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	models, fresh := cache.Get("test")
	require.True(t, fresh)
	assert.Equal(t, "after-close-model", models[0].ID)
}

func TestModelCacheSetOwnsLifecycleBeforeMemoryPublication(t *testing.T) {
	t.Parallel()

	cache := newFileBackedCache(t)
	cache.memoryMu.Lock()
	setDone := make(chan struct{})
	go func() {
		cache.Set("test", testModels("test"))
		close(setDone)
	}()
	requireLifecycleReaderActive(t, cache, setDone)

	clearDone := make(chan struct{})
	go func() {
		cache.Clear()
		close(clearDone)
	}()
	requireLifecycleWriterQueued(t, cache, clearDone)

	cache.memoryMu.Unlock()
	<-setDone
	<-clearDone
	assert.Zero(t, cache.Size())
	assert.NoFileExists(t, cache.getProviderFilePath("test"))
}

func TestModelCacheGetCompletesBeforeOverlappingClear(t *testing.T) {
	t.Parallel()

	cache := newFileBackedCache(t)
	cache.Set("test", testModels("test"))
	cache.memoryMu.Lock()
	delete(cache.memory, "test")
	cache.memoryMu.Unlock()

	providerLock := cache.getProviderLock("test")
	providerLock.Lock()
	type getResult struct {
		models []*types.ModelInfo
		fresh  bool
	}
	getResultCh := make(chan getResult, 1)
	getDone := make(chan struct{})
	go func() {
		models, fresh := cache.Get("test")
		getResultCh <- getResult{models: models, fresh: fresh}
		close(getDone)
	}()
	requireLifecycleReaderActive(t, cache, getDone)

	clearDone := make(chan struct{})
	go func() {
		cache.Clear()
		close(clearDone)
	}()
	requireLifecycleWriterQueued(t, cache, clearDone)

	providerLock.Unlock()
	<-getDone
	result := <-getResultCh
	<-clearDone
	require.True(t, result.fresh)
	require.Len(t, result.models, 1)
	assert.Equal(t, "test-model", result.models[0].ID)
	assert.Zero(t, cache.Size())
	assert.NoFileExists(t, cache.getProviderFilePath("test"))
}

func TestModelCacheSaveAfterCloseDoesNotPersist(t *testing.T) {
	t.Parallel()

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		cache := newFileBackedCache(t)
		require.NoError(t, cache.Close())
		cache.Set("test", testModels("test"))
		cache.saveToFile("direct", testModels("direct"))

		assert.Equal(t, 1, cache.Size())
		assert.NoFileExists(t, cache.getProviderFilePath("test"))
		assert.NoFileExists(t, cache.getProviderFilePath("direct"))
	})

	t.Run("modify", func(t *testing.T) {
		t.Parallel()

		cache := newFileBackedCache(t)
		cache.Set("test", testModels("before"))
		path := cache.getProviderFilePath("test")
		before, err := os.ReadFile(path)
		require.NoError(t, err)

		require.NoError(t, cache.Close())
		cache.Set("test", testModels("after"))

		after, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, before, after)
	})
}
