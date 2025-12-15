package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// ModelCache implements 3-tier caching: memory -> file -> fallback
type ModelCache struct {
	memory          *sync.Map // provider -> *CacheEntry
	filePath        string
	memoryTTL       time.Duration
	fileTTL         time.Duration
	enableFileCache bool
	fallback        map[string][]*types.ModelInfo
	mu              sync.RWMutex
}

// NewModelCache creates a new model cache
func NewModelCache(config DiscoveryConfig) *ModelCache {
	return &ModelCache{
		memory:          &sync.Map{},
		filePath:        expandPath(config.FileCachePath),
		memoryTTL:       config.CacheTTL,
		fileTTL:         config.FileCacheTTL,
		enableFileCache: config.EnableFileCache,
		fallback:        getFallbackModels(),
	}
}

// Get retrieves models from cache (L1 -> L2 -> L3)
func (c *ModelCache) Get(provider string) ([]*types.ModelInfo, bool) {
	// L1: Check memory cache
	if entry, ok := c.memory.Load(provider); ok {
		cached := entry.(*CacheEntry)
		if time.Since(cached.Timestamp) < c.memoryTTL {
			return cached.Models, true
		}
	}

	// L2: Check file cache (if enabled)
	if c.enableFileCache {
		if models, ok := c.loadFromFile(provider); ok {
			// Populate memory cache
			c.memory.Store(provider, &CacheEntry{
				Models:    models,
				Timestamp: time.Now(),
				Provider:  provider,
			})
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
	c.memory.Store(provider, entry)

	// L2: File cache (if enabled)
	if c.enableFileCache {
		c.saveToFile(provider, models)
	}
}

// loadFromFile loads models from persistent file cache
func (c *ModelCache) loadFromFile(provider string) ([]*types.ModelInfo, bool) {
	// Read file
	data, err := os.ReadFile(c.filePath)
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
	c.mu.Lock()
	defer c.mu.Unlock()

	// Read existing cache
	var fileCache FileCache
	data, err := os.ReadFile(c.filePath)
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return // Can't create directory, skip save
	}

	// Write atomically (write to temp, then rename)
	// Use 0600 for security (cache may contain API-related metadata)
	tempPath := c.filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return // Can't write, skip
	}
	if err := os.Rename(tempPath, c.filePath); err != nil {
		// Cleanup temp file on rename failure
		_ = os.Remove(tempPath)
	}
}

// Clear removes all cached entries
func (c *ModelCache) Clear() {
	c.memory = &sync.Map{}
	if c.enableFileCache {
		os.Remove(c.filePath)
	}
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
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
