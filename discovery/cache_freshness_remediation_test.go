package discovery

import (
	"encoding/json"
	"os"
	"testing"
	"testing/synctest"
	"time"
)

func TestAuditCachePromotionPreservesAcquisition(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"current", "legacy-shard", "monolithic"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				cache := newFileBackedCache(t)
				t.Cleanup(func() { _ = cache.Close() })
				cache.memoryTTL = time.Minute
				cache.fileTTL = time.Minute
				acquired := time.Now().Add(-59 * time.Second)
				entry := &CacheEntry{SchemaVersion: cacheSchemaVersion, Provider: "freshness-test", Timestamp: acquired, Models: testModels("freshness-test")}
				path := cache.getProviderFilePath(entry.Provider)
				var value any = entry
				switch source {
				case "legacy-shard":
					path = cache.getLegacyProviderFilePath(entry.Provider)
				case "monolithic":
					path = cache.filePath
					value = FileCache{Entries: map[string]*CacheEntry{entry.Provider: entry}}
				}
				data, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				if models, fresh := cache.Get(entry.Provider); !fresh || len(models) != 1 {
					t.Fatalf("near-expiry entry rejected: %v", models)
				}
				cache.memoryMu.RLock()
				promoted := cache.memory[entry.Provider].Timestamp
				cache.memoryMu.RUnlock()
				if !promoted.Equal(acquired) {
					t.Fatalf("promotion renewed timestamp: %v != %v", promoted, acquired)
				}
				synctest.Wait() // Finish any migration before advancing virtual time.
				<-time.After(2 * time.Second)
				if models, fresh := cache.Get(entry.Provider); fresh || len(models) != 0 {
					t.Fatalf("expired promotion served: fresh=%v models=%v", fresh, models)
				}
			})
		})
	}
}

func TestDiskPromotionReusesAgedShardUntilReloadAndOriginalExpiry(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		cache := newFileBackedCache(t)
		t.Cleanup(func() { _ = cache.Close() })
		cache.memoryTTL = time.Minute
		cache.fileTTL = 4 * time.Minute
		const provider = "aged-shard"
		acquired := time.Now().Add(-2 * time.Minute)
		path := cache.getProviderFilePath(provider)
		writeShard := func(id string) {
			t.Helper()
			data, err := json.Marshal(CacheEntry{
				SchemaVersion: cacheSchemaVersion,
				Provider:      provider,
				Timestamp:     acquired,
				Models:        testModels(id),
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		assertModel := func(want string, fresh bool) {
			t.Helper()
			models, gotFresh := cache.Get(provider)
			if gotFresh != fresh || len(models) != 1 || models[0].ID != want+"-model" {
				t.Fatalf("Get(%q) = %v, fresh=%v; want %q, fresh=%v", provider, models, gotFresh, want, fresh)
			}
		}

		writeShard("original")
		assertModel("original", true)
		writeShard("changed")
		assertModel("original", true) // A second lookup must not reread the shard.
		<-time.After(time.Minute)
		assertModel("changed", true) // File changes become visible within memoryTTL.
		<-time.After(time.Minute + time.Second)
		models, fresh := cache.Get(provider)
		if fresh || len(models) != 0 {
			t.Fatalf("original file expiry was extended: fresh=%v models=%v", fresh, models)
		}
	})
}

func TestAuditCachePromotionHonorsMemoryTTL(t *testing.T) {
	t.Parallel()
	cache := newFileBackedCache(t)
	t.Cleanup(func() { _ = cache.Close() })
	cache.memoryTTL = time.Minute
	cache.fileTTL = time.Hour
	acquired := time.Now().Add(-2 * time.Minute)
	entry := CacheEntry{SchemaVersion: cacheSchemaVersion, Provider: "tier-test", Timestamp: acquired, Models: testModels("tier-test")}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache.getProviderFilePath(entry.Provider), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, fresh := cache.Get(entry.Provider); !fresh {
		t.Fatal("file TTL was not applied")
	}
	cache.enableFileCache = false
	if _, fresh := cache.Get(entry.Provider); fresh {
		t.Fatal("memory tier renewed file acquisition time")
	}
}
