package fetchers

import (
	"crypto/sha256"
	"encoding/hex"
)

// accountKeyDiscriminator returns a short, stable, non-reversible token
// derived from an API key, used to scope on-disk model caches per
// credential so different accounts under the same provider name don't
// collide. Returns "" for an empty key (unscoped fallback).
func accountKeyDiscriminator(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:4])
}
