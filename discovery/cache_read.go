package discovery

import (
	"encoding/json"
	"os"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// Get retrieves models from cache (L1 -> L2 -> L3)
func (c *ModelCache) Get(provider string) ([]*types.ModelInfo, bool) {
	// L1: Check memory cache
	for _, lookup := range cacheLookupKeys(provider) {
		c.memoryMu.RLock()
		entry, ok := c.memory[lookup]
		c.memoryMu.RUnlock()
		if ok && time.Since(entry.Timestamp) < c.memoryTTL {
			if lookup != provider {
				c.memoryMu.Lock()
				c.memory[provider] = &CacheEntry{
					SchemaVersion: entry.SchemaVersion,
					Models:        entry.Models,
					Timestamp:     entry.Timestamp,
					Provider:      provider,
				}
				c.memoryMu.Unlock()
			}
			return cloneModels(entry.Models), true
		}
	}

	// L2: Check file cache (if enabled)
	if c.enableFileCache {
		if models, ok := c.loadFromFile(provider); ok {
			// Populate memory cache
			entry := &CacheEntry{
				Models:    models,
				Timestamp: time.Now(),
				Provider:  provider,
			}
			c.memoryMu.Lock()
			c.memory[provider] = entry
			c.memoryMu.Unlock()
			return cloneModels(models), true
		}
	}

	// L3: Return fallback (indicates stale/offline)
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, lookup := range cacheLookupKeys(provider) {
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
	for _, lookup := range cacheLookupKeys(provider) {
		c.memoryMu.RLock()
		entry := c.memory[lookup]
		c.memoryMu.RUnlock()
		if entry != nil && len(entry.Models) > 0 {
			return cloneModels(entry.Models)
		}
	}
	return nil
}

// Set stores models in cache (L1 + L2)
func (c *ModelCache) Set(provider string, models []*types.ModelInfo) {
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
	if c.enableFileCache {
		c.saveToFile(provider, models)
	}
}

// loadFromFile loads models from persistent file cache
func (c *ModelCache) loadFromFile(provider string) ([]*types.ModelInfo, bool) {
	for _, lookup := range cacheLookupKeys(provider) {
		// Use per-provider read lock for consistency
		lock := c.getProviderLock(lookup)
		lock.RLock()

		// Try provider-specific file first
		providerPath := c.getProviderFilePath(lookup)
		data, err := os.ReadFile(providerPath) // #nosec G304 - path validated via ValidatePath
		if err == nil {
			lock.RUnlock()
			entry, ok := c.decodeProviderShard(data, lookup)
			if !ok {
				return nil, false
			}
			if time.Since(entry.Timestamp) > c.fileTTL {
				continue
			}
			if lookup != provider {
				c.scheduleMigration(provider, entry)
			}
			return entry.Models, true
		}
		if !os.IsNotExist(err) {
			lock.RUnlock()
			return nil, false
		}

		legacyPath := c.getLegacyProviderFilePath(lookup)
		data, err = os.ReadFile(legacyPath) // #nosec G304 - path validated via ValidatePath
		lock.RUnlock()
		if err == nil {
			entry, ok := c.decodeProviderShard(data, lookup)
			if !ok {
				return nil, false
			}
			if time.Since(entry.Timestamp) > c.fileTTL {
				continue
			}
			c.scheduleMigration(provider, entry)
			return entry.Models, true
		}
		if !os.IsNotExist(err) {
			return nil, false
		}
	}

	// Fallback to monolithic file for backward compatibility
	data, err := os.ReadFile(c.filePath) // #nosec G304 - path validated via ValidatePath
	if err != nil {
		return nil, false // File doesn't exist or can't be read
	}

	// Parse JSON
	var fileCache FileCache
	if err := json.Unmarshal(data, &fileCache); err != nil {
		return nil, false // Invalid JSON
	}

	for _, lookup := range cacheLookupKeys(provider) {
		entry, ok := fileCache.Entries[lookup]
		if !ok {
			continue
		}
		if entry.Provider != "" && entry.Provider != lookup {
			continue
		}

		// Check TTL
		if time.Since(entry.Timestamp) > c.fileTTL {
			continue
		}

		// Migrate to provider-specific file for future reads
		c.scheduleMigration(provider, entry)
		return entry.Models, true
	}

	return nil, false
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

func (c *ModelCache) scheduleMigration(provider string, entry *CacheEntry) {
	c.muClosed.RLock()
	if c.closed {
		c.muClosed.RUnlock()
		return
	}
	gen := c.clearGen
	c.wg.Add(1)
	c.muClosed.RUnlock()

	go func() {
		defer c.wg.Done()

		entryCopy := *entry
		entryCopy.Models = cloneModels(entry.Models)
		entryCopy.Provider = provider
		c.migrateToSharded(provider, &entryCopy, gen)
	}()
}
