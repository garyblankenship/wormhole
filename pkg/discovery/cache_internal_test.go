package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testModels(provider string) []*types.ModelInfo {
	return []*types.ModelInfo{{
		ID:       provider + "-model",
		Name:     provider + " Model",
		Provider: provider,
		Capabilities: []types.ModelCapability{
			types.CapabilityText,
		},
	}}
}

func newFileBackedCache(t *testing.T) *ModelCache {
	t.Helper()
	return NewModelCache(DiscoveryConfig{
		CacheTTL:        time.Hour,
		FileCacheTTL:    time.Hour,
		FileCachePath:   filepath.Join(t.TempDir(), "models.json"),
		EnableFileCache: true,
	})
}

func TestModelCacheFileBackedSetGetClear(t *testing.T) {
	cache := newFileBackedCache(t)
	models := testModels("test")

	cache.Set("test", models)
	assert.Equal(t, 1, cache.Size())

	cached, fresh := cache.Get("test")
	require.True(t, fresh)
	require.Len(t, cached, 1)
	assert.Equal(t, "test-model", cached[0].ID)

	providerPath := cache.getProviderFilePath("test")
	require.FileExists(t, providerPath)

	cache.memory = make(map[string]*CacheEntry)
	cached, fresh = cache.Get("test")
	require.True(t, fresh)
	require.Len(t, cached, 1)
	assert.Equal(t, "test-model", cached[0].ID)

	cache.Clear()
	assert.Equal(t, 0, cache.Size())
	assert.NoFileExists(t, providerPath)
}

func TestModelCacheLoadFromMonolithicFileMigrates(t *testing.T) {
	cache := newFileBackedCache(t)
	entry := &CacheEntry{
		Models:    testModels("legacy"),
		Timestamp: time.Now(),
		Provider:  "legacy",
	}
	fileCache := FileCache{
		Version: "1",
		Updated: time.Now(),
		Entries: map[string]*CacheEntry{
			"legacy": entry,
		},
	}
	data, err := json.Marshal(fileCache)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cache.filePath, data, 0o600))

	models, ok := cache.loadFromFile("legacy")
	require.True(t, ok)
	require.Len(t, models, 1)

	cache.migrateToSharded("legacy", entry)
	assert.FileExists(t, cache.getProviderFilePath("legacy"))
}

func TestModelCacheExpiredInvalidAndFallbackPaths(t *testing.T) {
	cache := newFileBackedCache(t)
	expired := CacheEntry{
		Models:    testModels("expired"),
		Timestamp: time.Now().Add(-2 * time.Hour),
		Provider:  "expired",
	}
	data, err := json.Marshal(expired)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cache.getProviderFilePath("expired"), data, 0o600))

	models, ok := cache.loadFromFile("expired")
	assert.False(t, ok)
	assert.Nil(t, models)

	require.NoError(t, os.WriteFile(cache.getProviderFilePath("bad-json"), []byte("{"), 0o600))
	models, ok = cache.loadFromFile("bad-json")
	assert.False(t, ok)
	assert.Nil(t, models)

	fallback, fresh := cache.Get("openai")
	assert.False(t, fresh)
	assert.NotEmpty(t, fallback)

	missing, fresh := cache.Get("missing")
	assert.False(t, fresh)
	assert.Nil(t, missing)
}

func TestModelCacheProviderPathsJournalAndCleanup(t *testing.T) {
	cache := newFileBackedCache(t)
	assert.Contains(t, cache.getProviderFilePath("openrouter/../model"), "openrouter___model")
	assert.Same(t, cache.getProviderLock("openai"), cache.getProviderLock("openai"))

	models := testModels("journal")
	require.NoError(t, cache.appendToJournal("journal/provider", models))
	journalPath := cache.filePath + ".journal_provider.journal"
	data, err := os.ReadFile(journalPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), computeChecksum(models))

	cache.memory["fresh"] = &CacheEntry{Timestamp: time.Now(), Provider: "fresh"}
	cache.memory["expired"] = &CacheEntry{Timestamp: time.Now().Add(-2 * time.Hour), Provider: "expired"}
	cache.cleanupExpired()
	assert.Contains(t, cache.memory, "fresh")
	assert.NotContains(t, cache.memory, "expired")
}

func TestModelCacheStartCleanupCloseAndExpandPath(t *testing.T) {
	cache := NewModelCache(DiscoveryConfig{
		CacheTTL:        time.Nanosecond,
		FileCacheTTL:    time.Hour,
		FileCachePath:   filepath.Join(t.TempDir(), "models.json"),
		EnableFileCache: false,
	})
	cache.Set("test", testModels("test"))
	cache.StartCleanup(time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, cache.Close())
	require.NoError(t, cache.Close())
	assert.Equal(t, 0, cache.Size())

	validated, err := expandPath("../bad")
	require.NoError(t, err)
	assert.Equal(t, "wormhole-cache.json", filepath.Base(validated))

	homePath, err := expandPath("~/.wormhole/test-models.json")
	require.NoError(t, err)
	assert.NotContains(t, homePath, "~")
}
