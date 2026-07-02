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

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/types"
)

// cacheSchemaVersion is stamped into every persisted CacheEntry shard. Bump
// this when the on-disk CacheEntry shape changes so old shards are treated
// as a cache miss instead of being blindly unmarshaled into a new shape.
const cacheSchemaVersion = 1

// writeShardAtomic writes data to path atomically: a unique per-call temp
// file (avoiding the collision a fixed shared ".tmp" name would hit across
// concurrent processes/goroutines writing the same path), fsync'd before
// the rename so a crash can't leave a truncated shard on disk.
func writeShardAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tmp.Name()
	defer func() {
		_ = os.Remove(tempPath) // #nosec G304 - path validated via ValidatePath -- no-op once renamed
	}()

	if err := tmp.Chmod(0600); err != nil { // #nosec G304 - path validated via ValidatePath
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path) // #nosec G304 - path validated via ValidatePath
}

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
	// Sanitize provider name for file usage
	safeProvider := strings.ReplaceAll(provider, "/", "_")
	safeProvider = strings.ReplaceAll(safeProvider, "..", "_")
	safeProvider = strings.ReplaceAll(safeProvider, "\\", "_")

	// Extract directory and base name from original filePath
	dir := filepath.Dir(c.filePath)
	base := filepath.Base(c.filePath)
	// Remove extension if present
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	// Construct provider-specific filename: base-provider.ext
	providerBase := fmt.Sprintf("%s-%s%s", base, safeProvider, ext)
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

	// Abort if cache has been closed, or cleared since this goroutine was spawned
	c.muClosed.RLock()
	abort := c.closed || c.clearGen != gen
	c.muClosed.RUnlock()
	if abort {
		return
	}

	// Write atomically (unique temp path + fsync before rename)
	if err := writeShardAtomic(providerPath, data); err != nil {
		return // Can't write, skip
	}
}

// Get retrieves models from cache (L1 -> L2 -> L3)
func (c *ModelCache) Get(provider string) ([]*types.ModelInfo, bool) {
	// L1: Check memory cache
	c.memoryMu.RLock()
	entry, ok := c.memory[provider]
	c.memoryMu.RUnlock()
	if ok && time.Since(entry.Timestamp) < c.memoryTTL {
		return entry.Models, true
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
			return models, true
		}
	}

	// L3: Return fallback (indicates stale/offline)
	c.mu.RLock()
	defer c.mu.RUnlock()
	if models, ok := c.fallback[provider]; ok {
		return models, false // false = using fallback
	}

	return nil, false
}

// Set stores models in cache (L1 + L2)
func (c *ModelCache) Set(provider string, models []*types.ModelInfo) {
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
	// Use per-provider read lock for consistency
	lock := c.getProviderLock(provider)
	lock.RLock()
	defer lock.RUnlock()

	// Try provider-specific file first
	providerPath := c.getProviderFilePath(provider)
	data, err := os.ReadFile(providerPath) // #nosec G304 - path validated via ValidatePath
	if err == nil {
		// Parse provider-specific JSON (single CacheEntry)
		var entry CacheEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, false // Invalid JSON
		}
		// Reject shards from an incompatible schema version instead of
		// trusting a shape that may have since changed
		if entry.SchemaVersion != cacheSchemaVersion {
			return nil, false // Schema mismatch, treat as cache miss
		}
		// Check TTL
		if time.Since(entry.Timestamp) > c.fileTTL {
			return nil, false // Expired
		}
		return entry.Models, true
	}

	// Fallback to monolithic file for backward compatibility
	data, err = os.ReadFile(c.filePath) // #nosec G304 - path validated via ValidatePath
	if err != nil {
		return nil, false // File doesn't exist or can't be read
	}

	// Parse JSON
	var fileCache FileCache
	if err := json.Unmarshal(data, &fileCache); err != nil {
		return nil, false // Invalid JSON
	}

	// Get entry for provider
	entry, ok := fileCache.Entries[provider]
	if !ok {
		return nil, false // Provider not in cache
	}

	// Check TTL
	if time.Since(entry.Timestamp) > c.fileTTL {
		return nil, false // Expired
	}

	// Migrate to provider-specific file for future reads
	c.muClosed.RLock()
	gen := c.clearGen
	c.muClosed.RUnlock()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.migrateToSharded(provider, entry, gen)
	}()

	return entry.Models, true
}

// saveToFile persists models to file cache
func (c *ModelCache) saveToFile(provider string, models []*types.ModelInfo) {
	// Get or create provider-specific lock
	lock := c.getProviderLock(provider)
	lock.Lock()
	defer lock.Unlock()

	// Use append-based journaling if enabled (experimental)
	if c.enableAppendJournal {
		_ = c.appendToJournal(provider, models) // Ignore errors for backward compatibility
	}

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

// appendToJournal appends cache updates to a journal file (experimental)
func (c *ModelCache) appendToJournal(provider string, models []*types.ModelInfo) error {
	// Sanitize provider name for file usage
	safeProvider := strings.ReplaceAll(provider, "/", "_")
	safeProvider = strings.ReplaceAll(safeProvider, "..", "_")
	safeProvider = strings.ReplaceAll(safeProvider, "\\", "_")
	journalPath := c.filePath + "." + safeProvider + ".journal"

	entry := JournalEntry{
		Provider:  provider,
		Models:    models,
		Timestamp: time.Now(),
		Checksum:  computeChecksum(models),
		Sequence:  time.Now().UnixNano(), // Simple monotonic sequence
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(journalPath)
	if err := os.MkdirAll(dir, 0750); err != nil { // #nosec G304 - path validated via ValidatePath
		return err
	}

	// Append with O_APPEND flag
	f, err := os.OpenFile(journalPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // #nosec G304 - path validated via ValidatePath
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Printf("warning: failed to close journal file: %v", closeErr)
		}
	}()

	// Write with newline separator
	if _, err := f.Write(data); err != nil {
		return err
	}
	if _, err := f.Write([]byte("\n")); err != nil {
		return err
	}

	return nil
}

// computeChecksum calculates SHA256 checksum of models data
func computeChecksum(models []*types.ModelInfo) string {
	data, err := json.Marshal(models)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// Clear removes all cached entries
func (c *ModelCache) Clear() {
	c.muClosed.Lock()
	c.clearGen++
	c.muClosed.Unlock()

	c.memoryMu.Lock()
	for k := range c.memory {
		delete(c.memory, k)
	}
	c.memoryMu.Unlock()
	if c.enableFileCache {
		// Remove monolithic file for backward compatibility
		if err := os.Remove(c.filePath); err != nil && !os.IsNotExist(err) {
			// Log warning - file removal failed for unexpected reason
			log.Printf("warning: failed to remove cache file %s: %v", c.filePath, err) // #nosec G304 - path validated via ValidatePath
		}
		// Remove provider-specific files
		c.clearProviderFiles()
	}
}

// clearProviderFiles removes all provider-specific cache files
func (c *ModelCache) clearProviderFiles() {
	dir := filepath.Dir(c.filePath)
	base := filepath.Base(c.filePath)
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	// Pattern: base-*.ext
	pattern := filepath.Join(dir, base+"-*"+ext)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return // No matches or error
	}
	for _, path := range matches {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("warning: failed to remove provider cache file %s: %v", path, err) // #nosec G304 - path validated via ValidatePath
		}
	}
}

// Size returns the number of entries in the memory cache
func (c *ModelCache) Size() int {
	c.memoryMu.RLock()
	defer c.memoryMu.RUnlock()
	return len(c.memory)
}

// StartCleanup starts a background goroutine that periodically removes expired entries
func (c *ModelCache) StartCleanup(interval time.Duration) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.cleanupExpired()
			case <-c.stopCh:
				return
			}
		}
	}()
}

// Close stops the cleanup goroutine and waits for it to finish
func (c *ModelCache) Close() error {
	c.muClosed.Lock()
	c.closed = true
	c.muClosed.Unlock()

	c.stopOnce.Do(func() {
		close(c.stopCh)
		c.wg.Wait()
	})
	return nil
}

// cleanupExpired removes expired entries from the memory cache
func (c *ModelCache) cleanupExpired() {
	c.memoryMu.Lock()
	defer c.memoryMu.Unlock()
	now := time.Now()
	for k, entry := range c.memory {
		if now.Sub(entry.Timestamp) > c.memoryTTL {
			delete(c.memory, k)
		}
	}
}

// expandPath expands ~ to home directory and validates the path.
// Returns a validated, safe path. If validation fails, returns a default safe path.
func expandPath(path string) (string, error) {
	// Expand ~/ prefix
	expanded := path
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			expanded = filepath.Join(home, path[2:])
		}
	}

	// Validate the path (no base restriction, but prevent traversal)
	validated, err := utils.ValidatePath(expanded, "")
	if err != nil {
		// Log warning and fallback to default path
		log.Printf("warning: invalid cache path %q: %v, using default", path, err)
		// Default to current directory with safe name
		defaultPath := "./wormhole-cache.json"
		validated, err = utils.ValidatePath(defaultPath, "")
		if err != nil {
			// This should never happen, but if it does, return error
			return "", fmt.Errorf("failed to validate default cache path: %w", err)
		}
		return validated, nil
	}
	return validated, nil
}

// getFallbackModels returns minimal hardcoded models for offline mode
func getFallbackModels() map[string][]*types.ModelInfo {
	return map[string][]*types.ModelInfo{
		"openai": {
			{
				ID:       "gpt-5",
				Name:     "GPT-5",
				Provider: "openai",
				Capabilities: []types.ModelCapability{
					types.CapabilityText,
					types.CapabilityChat,
					types.CapabilityFunctions,
					types.CapabilityStructured,
				},
				MaxTokens: 128000,
			},
			{
				ID:       "gpt-5-mini",
				Name:     "GPT-5 Mini",
				Provider: "openai",
				Capabilities: []types.ModelCapability{
					types.CapabilityText,
					types.CapabilityChat,
					types.CapabilityFunctions,
					types.CapabilityStructured,
				},
				MaxTokens: 128000,
			},
		},
		"anthropic": {
			{
				ID:       "claude-sonnet-4-5",
				Name:     "Claude Sonnet 4.5",
				Provider: "anthropic",
				Capabilities: []types.ModelCapability{
					types.CapabilityText,
					types.CapabilityChat,
					types.CapabilityFunctions,
					types.CapabilityStructured,
					types.CapabilityVision,
				},
				MaxTokens: 200000,
			},
		},
		"openrouter": {
			// OpenRouter is fully dynamic, no fallback needed
		},
		"ollama": {
			// Ollama models are user-specific, no fallback possible
		},
	}
}
