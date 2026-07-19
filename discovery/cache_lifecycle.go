package discovery

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
