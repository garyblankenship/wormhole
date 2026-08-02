package discovery

import (
	"encoding/json"
	"os"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// Get retrieves models from cache (L1 -> L2 -> L3)
func (c *ModelCache) Get(provider string) ([]*types.ModelInfo, bool) {
	c.muClosed.RLock()
	releaseLifecycle := true
	defer func() {
		if releaseLifecycle {
			c.muClosed.RUnlock()
		}
	}()

	// L1: Check memory cache
	c.memoryMu.RLock()
	entry, ok := c.memory[provider]
	c.memoryMu.RUnlock()
	if ok && time.Since(entry.Timestamp) < c.memoryTTL {
		return cloneModels(entry.Models), true
	}

	// L2: Check file cache (if enabled)
	if c.enableFileCache {
		if models, ok, migration := c.loadFromFileLifecycleHeld(provider); ok {
			// Populate memory cache
			entry := &CacheEntry{
				Models:    models,
				Timestamp: time.Now(),
				Provider:  provider,
			}
			c.memoryMu.Lock()
			c.memory[provider] = entry
			c.memoryMu.Unlock()
			if migration != nil {
				releaseLifecycle = !c.scheduleMigrationLifecycleHeld(provider, migration)
			}
			return cloneModels(models), true
		}
	}

	// L3: Return fallback (indicates stale/offline)
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, lookup := range fallbackLookupKeys(provider) {
		if models, ok := c.fallback[lookup]; ok {
			return cloneModels(models), false // false = using fallback
		}
	}

	return nil, false
}

// GetStale returns the most recent in-memory entry without applying its TTL.
// It is used only after a live discovery failure; normal cache reads continue
// to enforce freshness through Get.
func (c *ModelCache) GetStale(provider string) []*types.ModelInfo {
	c.memoryMu.RLock()
	entry := c.memory[provider]
	c.memoryMu.RUnlock()
	if entry != nil && len(entry.Models) > 0 {
		return cloneModels(entry.Models)
	}
	return nil
}

// Set stores models in cache (L1 + L2)
func (c *ModelCache) Set(provider string, models []*types.ModelInfo) {
	c.muClosed.RLock()
	defer c.muClosed.RUnlock()

	models = cloneModels(models)
	entry := &CacheEntry{
		Models:    models,
		Timestamp: time.Now(),
		Provider:  provider,
	}

	// L1: Memory cache
	c.memoryMu.Lock()
	c.memory[provider] = entry
	c.memoryMu.Unlock()

	// L2: File cache (if enabled)
	if c.enableFileCache && !c.closed {
		c.saveToFileLifecycleHeld(provider, models)
	}
}

// loadFromFile loads models from persistent file cache
func (c *ModelCache) loadFromFile(provider string) ([]*types.ModelInfo, bool) {
	c.muClosed.RLock()
	models, ok, migration := c.loadFromFileLifecycleHeld(provider)
	if migration != nil && c.scheduleMigrationLifecycleHeld(provider, migration) {
		return models, ok
	}
	c.muClosed.RUnlock()
	return models, ok
}

func (c *ModelCache) loadFromFileLifecycleHeld(provider string) ([]*types.ModelInfo, bool, *CacheEntry) {
	// Use a per-provider read lock for consistency.
	lock := c.getProviderLock(provider)
	lock.RLock()

	// Try the current provider-specific file first.
	providerPath := c.getProviderFilePath(provider)
	data, err := os.ReadFile(providerPath) // #nosec G304 - path validated via ValidatePath
	if err == nil {
		lock.RUnlock()
		entry, ok := c.decodeProviderShard(data, provider)
		if !ok {
			return nil, false, nil
		}
		if time.Since(entry.Timestamp) > c.fileTTL {
			return c.loadFromMonolithicLifecycleHeld(provider)
		}
		return entry.Models, true, nil
	}
	if !os.IsNotExist(err) {
		lock.RUnlock()
		return nil, false, nil
	}

	legacyPath := c.getLegacyProviderFilePath(provider)
	data, err = os.ReadFile(legacyPath) // #nosec G304 - path validated via ValidatePath
	lock.RUnlock()
	if err == nil {
		entry, ok := c.decodeProviderShard(data, provider)
		if !ok {
			return nil, false, nil
		}
		if time.Since(entry.Timestamp) > c.fileTTL {
			return c.loadFromMonolithicLifecycleHeld(provider)
		}
		return entry.Models, true, entry
	}
	if !os.IsNotExist(err) {
		return nil, false, nil
	}

	return c.loadFromMonolithicLifecycleHeld(provider)
}

// loadFromMonolithicLifecycleHeld reads the legacy cache using the same exact
// provider/account key requested by the caller.
func (c *ModelCache) loadFromMonolithicLifecycleHeld(provider string) ([]*types.ModelInfo, bool, *CacheEntry) {
	data, err := os.ReadFile(c.filePath) // #nosec G304 - path validated via ValidatePath
	if err != nil {
		return nil, false, nil // File doesn't exist or can't be read
	}

	// Parse JSON
	var fileCache FileCache
	if err := json.Unmarshal(data, &fileCache); err != nil {
		return nil, false, nil // Invalid JSON
	}

	entry, ok := fileCache.Entries[provider]
	if !ok || entry == nil || (entry.Provider != "" && entry.Provider != provider) {
		return nil, false, nil
	}

	// Check TTL
	if time.Since(entry.Timestamp) > c.fileTTL {
		return nil, false, nil
	}

	return entry.Models, true, entry
}

func (c *ModelCache) decodeProviderShard(data []byte, provider string) (*CacheEntry, bool) {
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}
	if entry.SchemaVersion != cacheSchemaVersion || entry.Provider != provider {
		return nil, false
	}
	return &entry, true
}

// scheduleMigrationLifecycleHeld transfers the caller's lifecycle read
// ownership to the migration goroutine. That preserves lifecycle -> provider
// lock ordering and prevents Clear or Close from overtaking admitted work.
func (c *ModelCache) scheduleMigrationLifecycleHeld(provider string, entry *CacheEntry) bool {
	if c.closed {
		return false
	}

	gen := c.clearGen
	entryCopy := *entry
	entryCopy.Models = cloneModels(entry.Models)
	entryCopy.Provider = provider
	c.wg.Add(1)

	go func() {
		defer c.muClosed.RUnlock()
		defer c.wg.Done()
		c.migrateToShardedLifecycleHeld(provider, &entryCopy, gen)
	}()
	return true
}
