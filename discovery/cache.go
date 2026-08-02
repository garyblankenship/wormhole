package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// ModelCache implements 3-tier caching: memory -> file -> fallback
type ModelCache struct {
	memory          map[string]*CacheEntry // provider -> *CacheEntry
	memoryMu        sync.RWMutex           // Protects memory map
	filePath        string
	memoryTTL       time.Duration
	fileTTL         time.Duration
	enableFileCache bool
	fallback        map[string][]*types.ModelInfo
	mu              sync.RWMutex             // Protects file operations
	fileLocks       map[string]*sync.RWMutex // Per-provider file locks
	fileLocksMu     sync.RWMutex             // Protects fileLocks map
	writeShard      func(string, []byte) error

	// Goroutine lifecycle management
	stopCh   chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
	muClosed sync.RWMutex // serializes Set, Get, and migration admission with Clear and Close
	closed   bool         // set only by Close(); prevents later persistent saves and migrations
	clearGen uint64       // incremented by Clear(); invalidates migrations not yet admitted
}

// NewModelCache creates a new model cache
func NewModelCache(config DiscoveryConfig) *ModelCache {
	filePath, err := expandPath(config.FileCachePath)
	if err != nil {
		// Log error and use default path
		log.Printf("warning: failed to expand cache path %q: %v, using default", config.FileCachePath, err)
		filePath = "./wormhole-cache.json"
	}

	return &ModelCache{
		memory:          make(map[string]*CacheEntry),
		filePath:        filePath,
		memoryTTL:       config.CacheTTL,
		fileTTL:         config.FileCacheTTL,
		enableFileCache: config.EnableFileCache,
		fallback:        getFallbackModels(),
		fileLocks:       make(map[string]*sync.RWMutex),
		writeShard:      writeShardAtomic,
		stopCh:          make(chan struct{}),
	}
}

// getProviderLock returns or creates a provider-specific lock
func (c *ModelCache) getProviderLock(provider string) *sync.RWMutex {
	c.fileLocksMu.RLock()
	lock, exists := c.fileLocks[provider]
	c.fileLocksMu.RUnlock()

	if exists {
		return lock
	}

	// Lock doesn't exist, create it
	c.fileLocksMu.Lock()
	defer c.fileLocksMu.Unlock()

	// Double-check after acquiring write lock
	lock, exists = c.fileLocks[provider]
	if !exists {
		lock = &sync.RWMutex{}
		c.fileLocks[provider] = lock
	}

	return lock
}

// getProviderFilePath returns the provider-specific cache file path
func (c *ModelCache) getProviderFilePath(provider string) string {
	// Keep a short readable prefix while using the full provider hash as the
	// shard identity. The hash prevents distinct names such as "a/b" and
	// "a_b" from sharing a file and racing under different provider locks.
	safeProvider := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, provider)
	if len(safeProvider) > 32 {
		safeProvider = safeProvider[:32]
	}
	providerHash := sha256.Sum256([]byte(provider))
	shardID := safeProvider + "-" + hex.EncodeToString(providerHash[:])

	return c.providerCachePath(shardID)
}

// getLegacyProviderFilePath returns the pre-hash shard path. It is read only
// for backward compatibility and must never be trusted without checking the
// provider identity stored inside the shard.
func (c *ModelCache) getLegacyProviderFilePath(provider string) string {
	safeProvider := strings.ReplaceAll(provider, "/", "_")
	safeProvider = strings.ReplaceAll(safeProvider, "..", "_")
	safeProvider = strings.ReplaceAll(safeProvider, "\\", "_")
	return c.providerCachePath(safeProvider)
}

func (c *ModelCache) providerCachePath(shardID string) string {
	dir := filepath.Dir(c.filePath)
	base := filepath.Base(c.filePath)
	// Remove extension if present
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	// Construct provider-specific filename: base-provider.ext
	providerBase := fmt.Sprintf("%s-%s%s", base, shardID, ext)
	return filepath.Join(dir, providerBase)
}

// migrateToSharded migrates a cache entry from monolithic file to provider-specific file
func (c *ModelCache) migrateToSharded(provider string, entry *CacheEntry, gen uint64) {
	c.muClosed.RLock()
	defer c.muClosed.RUnlock()

	if c.closed || c.clearGen != gen {
		return
	}
	c.migrateToShardedLifecycleHeld(provider, entry, gen)
}

// migrateToShardedLifecycleHeld commits a migration while its caller retains
// lifecycle read ownership. Lifecycle ownership must precede provider locks.
func (c *ModelCache) migrateToShardedLifecycleHeld(provider string, entry *CacheEntry, gen uint64) {
	if c.closed || c.clearGen != gen {
		return
	}

	// Use per-provider lock to prevent concurrent migration
	lock := c.getProviderLock(provider)
	lock.Lock()
	defer lock.Unlock()

	// Check if provider-specific file already exists (race condition)
	providerPath := c.getProviderFilePath(provider)
	if _, err := os.Stat(providerPath); err == nil {
		return // Already migrated
	}

	// Stamp current schema version (legacy monolithic entries predate this field)
	entry.SchemaVersion = cacheSchemaVersion

	// Marshal entry to JSON
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return // Can't marshal, skip migration
	}

	// Ensure directory exists
	dir := filepath.Dir(providerPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return // Can't create directory, skip
	}

	// Write atomically (unique temp path + fsync before rename)
	if err := c.writeShard(providerPath, data); err != nil {
		return // Can't write, skip
	}
}
