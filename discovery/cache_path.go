package discovery

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	errCachePathTraversal = errors.New("path traversal attempt detected")
	errInvalidCachePath   = errors.New("invalid path")
)

func expandPath(path string) (string, error) {
	expanded := path
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			expanded = filepath.Join(home, path[2:])
		}
	}

	validated, err := validateCachePath(expanded)
	if err == nil {
		return validated, nil
	}
	log.Printf("warning: invalid cache path %q: %v, using default", path, err)
	validated, err = validateCachePath("./wormhole-cache.json")
	if err != nil {
		return "", fmt.Errorf("failed to validate default cache path: %w", err)
	}
	return validated, nil
}

func validateCachePath(path string) (string, error) {
	if path == "" || strings.Contains(path, "\x00") {
		return "", errInvalidCachePath
	}
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return "", errCachePathTraversal
	}
	return cleaned, nil
}
