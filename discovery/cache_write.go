package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// saveToFile persists models to file cache
func (c *ModelCache) saveToFile(provider string, models []*types.ModelInfo) {
	// Get or create provider-specific lock
	lock := c.getProviderLock(provider)
	lock.Lock()
	defer lock.Unlock()

	// Create entry
	entry := &CacheEntry{
		SchemaVersion: cacheSchemaVersion,
		Models:        models,
		Timestamp:     time.Now(),
		Provider:      provider,
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return // Can't marshal, skip save
	}

	// Write to provider-specific file
	providerPath := c.getProviderFilePath(provider)
	// Ensure directory exists
	dir := filepath.Dir(providerPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return // Can't create directory, skip save
	}

	// Write atomically (unique temp path + fsync before rename)
	if err := writeShardAtomic(providerPath, data); err != nil {
		return // Can't write, skip
	}
}
