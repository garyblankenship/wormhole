package discovery

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
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
	t.Parallel()
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

func TestModelCacheClearRemovesOnlyOwnedShards(t *testing.T) {
	t.Parallel()

	cache := newFileBackedCache(t)
	writeEntry := func(path string, entry CacheEntry) {
		t.Helper()
		data, err := json.Marshal(entry)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))
	}
	ownedCurrent := cache.getProviderFilePath("current")
	ownedLegacy := cache.getLegacyProviderFilePath("legacy/provider")
	writeEntry(ownedCurrent, CacheEntry{
		SchemaVersion: cacheSchemaVersion,
		Models:        testModels("current"),
		Timestamp:     time.Now(),
		Provider:      "current",
	})
	writeEntry(ownedLegacy, CacheEntry{
		SchemaVersion: cacheSchemaVersion,
		Models:        testModels("legacy/provider"),
		Timestamp:     time.Now(),
		Provider:      "legacy/provider",
	})
	require.NoError(t, os.WriteFile(cache.filePath, []byte(`{"entries":{}}`), 0o600))

	dir := filepath.Dir(cache.filePath)
	backup := filepath.Join(dir, "models-backup.json")
	malformed := filepath.Join(dir, "models-malformed.json")
	mismatched := cache.getProviderFilePath("claimed")
	temporary := cache.getProviderFilePath("temporary") + ".tmp-1"
	directory := cache.getProviderFilePath("directory")
	oversized := cache.getProviderFilePath("oversized")
	require.NoError(t, os.WriteFile(backup, []byte("private backup"), 0o600))
	require.NoError(t, os.WriteFile(malformed, []byte(`{"provider":`), 0o600))
	writeEntry(mismatched, CacheEntry{
		SchemaVersion: cacheSchemaVersion,
		Models:        testModels("other"),
		Timestamp:     time.Now(),
		Provider:      "other",
	})
	require.NoError(t, os.WriteFile(temporary, []byte("temporary shard"), 0o600))
	require.NoError(t, os.Mkdir(directory, 0o700))
	oversizedData, err := json.Marshal(CacheEntry{
		SchemaVersion: cacheSchemaVersion,
		Models:        testModels("oversized"),
		Timestamp:     time.Now(),
		Provider:      "oversized",
	})
	require.NoError(t, err)
	oversizedData = append(oversizedData, bytes.Repeat([]byte(" "), maxProviderShardVerificationSize-len(oversizedData)+1)...)
	require.NoError(t, os.WriteFile(oversized, oversizedData, 0o600))

	symlink := cache.getProviderFilePath("symlink")
	require.NoError(t, os.Symlink(backup, symlink))

	cache.Clear()

	assert.NoFileExists(t, cache.filePath)
	assert.NoFileExists(t, ownedCurrent)
	assert.NoFileExists(t, ownedLegacy)
	assert.FileExists(t, backup)
	assert.FileExists(t, malformed)
	assert.FileExists(t, mismatched)
	assert.FileExists(t, temporary)
	assert.DirExists(t, directory)
	assert.FileExists(t, oversized)
	info, err := os.Lstat(symlink)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
}

func TestModelCacheLoadFromMonolithicFileMigrates(t *testing.T) {
	t.Parallel()
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

	cache.migrateToSharded("legacy", entry, 0)
	assert.FileExists(t, cache.getProviderFilePath("legacy"))
}

func TestModelCacheScopedKeyRejectsBaseEntryWithoutMigration(t *testing.T) {
	t.Parallel()

	cache := newFileBackedCache(t)
	entry := &CacheEntry{
		Models:    testModels("openai"),
		Timestamp: time.Now(),
		Provider:  "openai",
	}
	fileCache := FileCache{
		Version: "1",
		Updated: time.Now(),
		Entries: map[string]*CacheEntry{
			"openai": entry,
		},
	}
	data, err := json.Marshal(fileCache)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cache.filePath, data, 0o600))

	models, fresh := cache.Get("openai__acct1234")
	require.False(t, fresh)
	require.NotEmpty(t, models)
	for _, model := range models {
		assert.NotEqual(t, "openai-model", model.ID)
	}

	scopedPath := cache.getProviderFilePath("openai__acct1234")
	assert.NoFileExists(t, scopedPath)
}

func TestModelCacheScopedLookupIgnoresUnscopedRecords(t *testing.T) {
	t.Parallel()

	const provider = "openai"
	const scoped = "openai__acct1234"
	assertFallback := func(t *testing.T, cache *ModelCache) {
		t.Helper()
		models, fresh := cache.Get(scoped)
		assert.False(t, fresh)
		require.NotEmpty(t, models)
		for _, model := range models {
			assert.NotEqual(t, "private-model", model.ID)
		}
		assert.NoFileExists(t, cache.getProviderFilePath(scoped))
	}

	t.Run("memory", func(t *testing.T) {
		t.Parallel()
		cache := newFileBackedCache(t)
		cache.Set(provider, []*types.ModelInfo{{ID: "private-model"}})
		assertFallback(t, cache)
	})
	t.Run("stale memory", func(t *testing.T) {
		t.Parallel()
		cache := newFileBackedCache(t)
		cache.memory[provider] = &CacheEntry{Models: []*types.ModelInfo{{ID: "private-model"}}, Timestamp: time.Now().Add(-2 * time.Hour), Provider: provider}
		assert.Nil(t, cache.GetStale(scoped))
	})
	t.Run("current shard", func(t *testing.T) {
		t.Parallel()
		cache := newFileBackedCache(t)
		data, err := json.Marshal(CacheEntry{SchemaVersion: cacheSchemaVersion, Models: []*types.ModelInfo{{ID: "private-model"}}, Timestamp: time.Now(), Provider: provider})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(cache.getProviderFilePath(provider), data, 0o600))
		assertFallback(t, cache)
	})
	t.Run("legacy shard", func(t *testing.T) {
		t.Parallel()
		cache := newFileBackedCache(t)
		data, err := json.Marshal(CacheEntry{SchemaVersion: cacheSchemaVersion, Models: []*types.ModelInfo{{ID: "private-model"}}, Timestamp: time.Now(), Provider: provider})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(cache.getLegacyProviderFilePath(provider), data, 0o600))
		assertFallback(t, cache)
	})
	t.Run("monolithic", func(t *testing.T) {
		t.Parallel()
		cache := newFileBackedCache(t)
		data, err := json.Marshal(FileCache{Entries: map[string]*CacheEntry{
			provider: {Models: []*types.ModelInfo{{ID: "private-model"}}, Timestamp: time.Now(), Provider: provider},
		}})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(cache.filePath, data, 0o600))
		assertFallback(t, cache)
	})
}

func TestModelCacheScopedMonolithicMigrationStaysWithinScope(t *testing.T) {
	t.Parallel()

	cache := newFileBackedCache(t)
	const provider = "openai__acct1234"
	entry := &CacheEntry{Models: []*types.ModelInfo{{ID: "private-model"}}, Timestamp: time.Now(), Provider: provider}
	data, err := json.Marshal(FileCache{Entries: map[string]*CacheEntry{provider: entry}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cache.filePath, data, 0o600))

	models, fresh := cache.Get(provider)
	require.True(t, fresh)
	require.Len(t, models, 1)
	assert.Equal(t, "private-model", models[0].ID)
	cache.wg.Wait()
	assert.FileExists(t, cache.getProviderFilePath(provider))
	assert.NoFileExists(t, cache.getProviderFilePath("openai"))
}

func TestModelCacheScopedAndUnscopedExpiredShardsFallThroughToMonolithic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		legacy   bool
	}{
		{name: "unscoped current", provider: "openai"},
		{name: "unscoped legacy", provider: "openai", legacy: true},
		{name: "scoped current", provider: "openai__acct1234"},
		{name: "scoped legacy", provider: "openai__acct1234", legacy: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cache := newFileBackedCache(t)
			t.Cleanup(func() {
				require.NoError(t, cache.Close())
			})
			expired := CacheEntry{
				SchemaVersion: cacheSchemaVersion,
				Models:        []*types.ModelInfo{{ID: "expired-model"}},
				Timestamp:     time.Now().Add(-2 * time.Hour),
				Provider:      test.provider,
			}
			expiredData, err := json.Marshal(expired)
			require.NoError(t, err)
			shardPath := cache.getProviderFilePath(test.provider)
			if test.legacy {
				shardPath = cache.getLegacyProviderFilePath(test.provider)
			}
			require.NoError(t, os.WriteFile(shardPath, expiredData, 0o600))

			fresh := &CacheEntry{
				Models:    []*types.ModelInfo{{ID: "fresh-monolithic-model"}},
				Timestamp: time.Now(),
				Provider:  test.provider,
			}
			monolithicData, err := json.Marshal(FileCache{Entries: map[string]*CacheEntry{test.provider: fresh}})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(cache.filePath, monolithicData, 0o600))

			models, ok := cache.loadFromFile(test.provider)
			require.True(t, ok)
			require.Len(t, models, 1)
			assert.Equal(t, "fresh-monolithic-model", models[0].ID)
		})
	}
}

func TestModelCacheExpiredInvalidAndFallbackPaths(t *testing.T) {
	t.Parallel()
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

func TestModelCacheProviderPathsAndCleanup(t *testing.T) {
	t.Parallel()
	cache := newFileBackedCache(t)
	assert.Contains(t, cache.getProviderFilePath("openrouter/../model"), "openrouter____model-")
	assert.NotEqual(t, cache.getProviderFilePath("a/b"), cache.getProviderFilePath("a_b"))
	assert.Same(t, cache.getProviderLock("openai"), cache.getProviderLock("openai"))

	cache.memory["fresh"] = &CacheEntry{Timestamp: time.Now(), Provider: "fresh"}
	cache.memory["expired"] = &CacheEntry{Timestamp: time.Now().Add(-2 * time.Hour), Provider: "expired"}
	cache.cleanupExpired()
	assert.Contains(t, cache.memory, "fresh")
	assert.NotContains(t, cache.memory, "expired")
}

func TestModelCacheRejectsShardForDifferentProvider(t *testing.T) {
	t.Parallel()
	cache := newFileBackedCache(t)
	entry := CacheEntry{
		SchemaVersion: cacheSchemaVersion,
		Models:        testModels("a_b"),
		Timestamp:     time.Now(),
		Provider:      "a_b",
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cache.getProviderFilePath("a/b"), data, 0o600))

	models, ok := cache.loadFromFile("a/b")
	assert.False(t, ok)
	assert.Nil(t, models)
}

func TestModelCacheLoadsAndMigratesLegacyProviderShard(t *testing.T) {
	t.Parallel()
	cache := newFileBackedCache(t)
	entry := CacheEntry{
		SchemaVersion: cacheSchemaVersion,
		Models:        testModels("legacy/provider"),
		Timestamp:     time.Now(),
		Provider:      "legacy/provider",
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cache.getLegacyProviderFilePath("legacy/provider"), data, 0o600))

	models, ok := cache.loadFromFile("legacy/provider")
	require.True(t, ok)
	require.Len(t, models, 1)
	assert.Equal(t, "legacy/provider-model", models[0].ID)
	require.Eventually(t, func() bool {
		_, err := os.Stat(cache.getProviderFilePath("legacy/provider"))
		return err == nil
	}, time.Second, 10*time.Millisecond)
}

func TestModelCacheRejectsCollidingLegacyProviderShard(t *testing.T) {
	t.Parallel()
	cache := newFileBackedCache(t)
	require.Equal(t, cache.getLegacyProviderFilePath("a/b"), cache.getLegacyProviderFilePath("a_b"))
	entry := CacheEntry{
		SchemaVersion: cacheSchemaVersion,
		Models:        testModels("a_b"),
		Timestamp:     time.Now(),
		Provider:      "a_b",
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cache.getLegacyProviderFilePath("a/b"), data, 0o600))

	models, ok := cache.loadFromFile("a/b")
	assert.False(t, ok)
	assert.Nil(t, models)
	assert.NoFileExists(t, cache.getProviderFilePath("a/b"))
}

func TestModelCacheCollidingLegacyNamesUseIndependentShards(t *testing.T) {
	t.Parallel()
	cache := newFileBackedCache(t)
	providers := []string{"a/b", "a_b"}

	var wg sync.WaitGroup
	for _, provider := range providers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.Set(provider, testModels(provider))
		}()
	}
	wg.Wait()

	cache.memoryMu.Lock()
	cache.memory = make(map[string]*CacheEntry)
	cache.memoryMu.Unlock()
	for _, provider := range providers {
		models, fresh := cache.Get(provider)
		require.True(t, fresh)
		require.Len(t, models, 1)
		assert.Equal(t, provider+"-model", models[0].ID)
	}
}

func TestModelCacheClonesInputsOutputsAndFallbacks(t *testing.T) {
	t.Parallel()
	cache := NewModelCache(DiscoveryConfig{
		CacheTTL:        time.Hour,
		FileCacheTTL:    time.Hour,
		FileCachePath:   filepath.Join(t.TempDir(), "models.json"),
		EnableFileCache: false,
	})
	models := []*types.ModelInfo{{
		ID:           "original",
		Provider:     "test",
		Cost:         &types.ModelCost{Currency: "USD", InputTokens: 1},
		Capabilities: []types.ModelCapability{types.CapabilityText},
		Constraints: map[string]any{
			"nested": map[string]any{"values": []any{"original"}},
		},
	}}

	cache.Set("test", models)
	models[0].ID = "mutated-input"
	models[0].Cost.Currency = "EUR"
	models[0].Capabilities[0] = types.CapabilityAudio
	models[0].Constraints["nested"].(map[string]any)["values"].([]any)[0] = "mutated-input"

	first, fresh := cache.Get("test")
	require.True(t, fresh)
	require.Len(t, first, 1)
	assert.Equal(t, "original", first[0].ID)
	assert.Equal(t, "USD", first[0].Cost.Currency)
	assert.Equal(t, types.CapabilityText, first[0].Capabilities[0])
	assert.Equal(t, "original", first[0].Constraints["nested"].(map[string]any)["values"].([]any)[0])

	first[0].ID = "mutated-output"
	first[0].Cost.Currency = "GBP"
	first[0].Capabilities[0] = types.CapabilityVision
	first[0].Constraints["nested"].(map[string]any)["values"].([]any)[0] = "mutated-output"

	second, fresh := cache.Get("test")
	require.True(t, fresh)
	assert.Equal(t, "original", second[0].ID)
	assert.Equal(t, "USD", second[0].Cost.Currency)
	assert.Equal(t, types.CapabilityText, second[0].Capabilities[0])
	assert.Equal(t, "original", second[0].Constraints["nested"].(map[string]any)["values"].([]any)[0])

	fallback, fresh := cache.Get("openai")
	require.False(t, fresh)
	require.NotEmpty(t, fallback)
	fallback[0].ID = "mutated-fallback"
	fallback[0].Capabilities[0] = types.CapabilityAudio

	fallbackAgain, fresh := cache.Get("openai")
	require.False(t, fresh)
	require.NotEmpty(t, fallbackAgain)
	assert.NotEqual(t, "mutated-fallback", fallbackAgain[0].ID)
	assert.NotEqual(t, types.CapabilityAudio, fallbackAgain[0].Capabilities[0])
}

func TestModelCacheStartCleanupCloseAndExpandPath(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
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
	})

	validated, err := expandPath("../bad")
	require.NoError(t, err)
	assert.Equal(t, "wormhole-cache.json", filepath.Base(validated))

	homePath, err := expandPath("~/.wormhole/test-models.json")
	require.NoError(t, err)
	assert.NotContains(t, homePath, "~")
}

func TestModelCacheLoadFromFileDoesNotMigrateAfterClose(t *testing.T) {
	t.Parallel()

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

	require.NoError(t, cache.Close())

	models, ok := cache.loadFromFile("legacy")
	require.True(t, ok)
	require.Len(t, models, 1)
	assert.NoFileExists(t, cache.getProviderFilePath("legacy"))
}

func TestModelCacheClearDoesNotResurrectMigratedShards(t *testing.T) {
	t.Parallel()

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

	cache.Clear()
	require.NoError(t, cache.Close())

	assert.NoFileExists(t, cache.filePath)
	assert.NoFileExists(t, cache.getProviderFilePath("legacy"))
}
