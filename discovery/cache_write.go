package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

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
