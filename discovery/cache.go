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

	"github.com/garyblankenship/wormhole/v2/types"
)

// ModelCache implements 3-tier caching: memory -> file -> fallback
type ModelCache struct {
	memory              map[string]*CacheEntry // provider -> *CacheEntry
	memoryMu            sync.RWMutex           // Protects memory map
	filePath            string
	memoryTTL           time.Duration
	fileTTL             time.Duration
	enableFileCache     bool
	enableAppendJournal bool // Experimental: use append-based journaling
	fallback            map[string][]*types.ModelInfo
	mu                  sync.RWMutex             // Protects file operations
	fileLocks           map[string]*sync.RWMutex // Per-provider file locks
	fileLocksMu         sync.RWMutex             // Protects fileLocks map

	// Goroutine lifecycle management
	stopCh   chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
	muClosed sync.RWMutex // protects closed and clearGen
	closed   bool         // set only by Close(); permanently aborts in-flight migrations
	clearGen uint64       // incremented by Clear(); aborts migrations spawned before this generation
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
		memory:              make(map[string]*CacheEntry),
		filePath:            filePath,
		memoryTTL:           config.CacheTTL,
		fileTTL:             config.FileCacheTTL,
		enableFileCache:     config.EnableFileCache,
		enableAppendJournal: false, // Disabled by default for compatibility
		fallback:            getFallbackModels(),
		fileLocks:           make(map[string]*sync.RWMutex),
		stopCh:              make(chan struct{}),
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

	// Serialize writes against Clear/Close so they cannot resurrect shards after
	// deletion or race WaitGroup shutdown.
	c.muClosed.RLock()
	abort := c.closed || c.clearGen != gen
	if abort {
		c.muClosed.RUnlock()
		return
	}
	defer c.muClosed.RUnlock()

	// Write atomically (unique temp path + fsync before rename)
	if err := writeShardAtomic(providerPath, data); err != nil {
		return // Can't write, skip
	}
}
