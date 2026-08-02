package discovery

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxProviderShardVerificationSize = 32 * 1024 * 1024

// Clear removes all cached entries
func (c *ModelCache) Clear() {
	c.muClosed.Lock()
	defer c.muClosed.Unlock()

	c.clearGen++

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

// clearProviderFiles removes only provider-specific cache files whose complete
// contents prove they belong to this cache. Prefix matching alone is unsafe:
// the cache directory can contain unrelated user files with the same prefix.
func (c *ModelCache) clearProviderFiles() {
	dir := filepath.Dir(c.filePath)
	base := filepath.Base(c.filePath)
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("warning: failed to list provider cache directory %s: %v", dir, err) // #nosec G304 - path validated via ValidatePath
		return
	}
	for _, entry := range entries {
		if !isProviderCacheFilename(entry.Name(), base, ext) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if !c.ownsProviderShard(path) {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("warning: failed to remove provider cache file %s: %v", path, err) // #nosec G304 - path validated via ValidatePath
		}
	}
}

func isProviderCacheFilename(name, base, ext string) bool {
	return strings.HasPrefix(name, base+"-") && strings.HasSuffix(name, ext)
}

// ownsProviderShard returns true only for a complete, current-schema shard
// whose embedded provider identity maps back to this exact current or legacy
// filename. It deliberately treats malformed and ambiguous files as user data.
func (c *ModelCache) ownsProviderShard(path string) bool {
	file, err := os.Open(path) // #nosec G304 - path validated via ValidatePath
	if err != nil {
		return false
	}
	defer func() {
		_ = file.Close()
	}()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxProviderShardVerificationSize {
		return false
	}

	var entry CacheEntry
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&entry); err != nil || entry.SchemaVersion != cacheSchemaVersion || entry.Provider == "" {
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return false
	}

	cleanedPath := filepath.Clean(path)
	return cleanedPath == filepath.Clean(c.getProviderFilePath(entry.Provider)) ||
		cleanedPath == filepath.Clean(c.getLegacyProviderFilePath(entry.Provider))
}

// Size returns the number of entries in the memory cache
func (c *ModelCache) Size() int {
	c.memoryMu.RLock()
	defer c.memoryMu.RUnlock()
	return len(c.memory)
}

// StartCleanup starts a background goroutine that periodically removes expired entries.
// A nonpositive interval disables cleanup.
func (c *ModelCache) StartCleanup(interval time.Duration) {
	if !c.admitCleanup(interval) {
		return
	}
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

// admitCleanup reserves a cleanup worker while lifecycle admission is held so
// Close cannot begin waiting before the worker is accounted for.
func (c *ModelCache) admitCleanup(interval time.Duration) bool {
	if interval <= 0 {
		return false
	}
	c.muClosed.RLock()
	defer c.muClosed.RUnlock()

	if c.closed {
		return false
	}
	c.wg.Add(1)
	return true
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
