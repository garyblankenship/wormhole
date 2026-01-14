package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/internal/utils"
	"github.com/garyblankenship/wormhole/pkg/types"
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
	stopCh  chan struct{}
	wg      sync.WaitGroup
	stopOnce sync.Once
}

// NewModelCache creates a new model cache
func NewModelCache(config DiscoveryConfig) *ModelCache {
	return &ModelCache{
		memory:              make(map[string]*CacheEntry),
		filePath:            expandPath(config.FileCachePath),
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

	// Read file
	data, err := os.ReadFile(c.filePath) // #nosec G304 - path validated via ValidatePath
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
		// Still update main cache file for backward compatibility
	}

	// Read existing cache
	var fileCache FileCache
	data, err := os.ReadFile(c.filePath) // #nosec G304 - path validated via ValidatePath
	if err == nil {
		// File exists, parse it (ignore unmarshal errors, will reinitialize)
		_ = json.Unmarshal(data, &fileCache)
	}

	// Initialize if needed
	if fileCache.Entries == nil {
		fileCache.Entries = make(map[string]*CacheEntry)
	}
	fileCache.Version = "1.0"
	fileCache.Updated = time.Now()

	// Update entry
	fileCache.Entries[provider] = &CacheEntry{
		Models:    models,
		Timestamp: time.Now(),
		Provider:  provider,
	}

	// Marshal to JSON
	data, err = json.MarshalIndent(fileCache, "", "  ")
	if err != nil {
		return // Can't marshal, skip save
	}

	// Ensure directory exists
	dir := filepath.Dir(c.filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return // Can't create directory, skip save
	}

	// Write atomically (write to temp, then rename)
	// Use 0600 for security (cache may contain API-related metadata)
	tempPath := c.filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil { // #nosec G304 - path validated via ValidatePath
		return // Can't write, skip
	}
	if err := os.Rename(tempPath, c.filePath); err != nil { // #nosec G304 - path validated via ValidatePath
		// Cleanup temp file on rename failure
		_ = os.Remove(tempPath) // #nosec G304 - path validated via ValidatePath
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
	defer f.Close()

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

// recoverFromJournal recovers cache state from journal files
func (c *ModelCache) recoverFromJournal() error {
	// Get all journal files in the cache directory
	dir := filepath.Dir(c.filePath)
	pattern := filepath.Join(dir, "*.journal")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		return nil // No journal files to recover from
	}

	// Process each journal file
	for _, journalPath := range matches {
		if err := c.processJournalFile(journalPath); err != nil {
			// Log error but continue with other journals
			// In production, you'd want proper logging here
			_ = err
		}
	}

	return nil
}

// processJournalFile reads and validates a single journal file
func (c *ModelCache) processJournalFile(journalPath string) error {
	data, err := os.ReadFile(journalPath) // #nosec G304 - path validated via ValidatePath
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	validEntries := make([]JournalEntry, 0, len(lines))

	// Validate each line
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry JournalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Invalid JSON, skip this entry
			continue
		}

		// Validate checksum
		expectedChecksum := computeChecksum(entry.Models)
		if entry.Checksum != "" && entry.Checksum != expectedChecksum {
			// Checksum mismatch, skip this entry
			continue
		}

		validEntries = append(validEntries, entry)
	}

	if len(validEntries) == 0 {
		return nil // No valid entries found
	}

	// Apply the most recent valid entry for each provider
	// For simplicity, we take the last entry per provider (highest sequence)
	latestByProvider := make(map[string]JournalEntry)
	for _, entry := range validEntries {
		if existing, exists := latestByProvider[entry.Provider]; !exists || entry.Sequence > existing.Sequence {
			latestByProvider[entry.Provider] = entry
		}
	}

	// Update cache with recovered entries
	for provider, entry := range latestByProvider {
		cacheEntry := &CacheEntry{
			Models:    entry.Models,
			Timestamp: entry.Timestamp,
			Provider:  provider,
		}

		// Update memory cache
		c.memoryMu.Lock()
		c.memory[provider] = cacheEntry
		c.memoryMu.Unlock()

		// Update file cache
		c.saveToFile(provider, entry.Models)
	}

	return nil
}

// Clear removes all cached entries
func (c *ModelCache) Clear() {
	c.memoryMu.Lock()
	for k := range c.memory {
		delete(c.memory, k)
	}
	c.memoryMu.Unlock()
	if c.enableFileCache {
		if err := os.Remove(c.filePath); err != nil && !os.IsNotExist(err) {
			// Log warning - file removal failed for unexpected reason
			log.Printf("warning: failed to remove cache file %s: %v", c.filePath, err) // #nosec G304 - path validated via ValidatePath
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
func expandPath(path string) string {
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
			// This should never happen, but if it does, panic
			panic("failed to validate default cache path: " + err.Error())
		}
		return validated
	}
	return validated
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
