// Package utils provides utility functions for the Wormhole SDK.
// This file contains path validation and sanitization utilities.
package utils

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrPathTraversal indicates a path traversal attempt was detected.
	ErrPathTraversal = errors.New("path traversal attempt detected")
	// ErrPathOutsideBase indicates a path is outside the allowed base directory.
	ErrPathOutsideBase = errors.New("path is outside allowed base directory")
	// ErrInvalidPath indicates the path is invalid (empty, contains null bytes, etc.)
	ErrInvalidPath = errors.New("invalid path")
)

// SanitizePath cleans and normalizes a path, resolving any . or .. components.
// It returns the cleaned path using filepath.Clean.
// Note: This does NOT prevent path traversal attacks on its own.
// Use ValidatePath to ensure the path is safe.
func SanitizePath(path string) string {
	return filepath.Clean(path)
}

// ValidatePath validates a path for safety:
//  1. Checks for null bytes and empty path
//  2. Ensures the path does not contain path traversal sequences after cleaning
//  3. Optionally ensures the path is within allowedBase directory
//
// Parameters:
//   - path: The path to validate
//   - allowedBase: If non-empty, ensures the path is within this directory
//
// Returns:
//   - The cleaned, absolute path if validation passes
//   - An error if the path is invalid or unsafe
func ValidatePath(path, allowedBase string) (string, error) {
	// Check for empty path
	if path == "" {
		return "", ErrInvalidPath
	}
	// Check for null bytes (indicates potential injection)
	if strings.Contains(path, "\x00") {
		return "", ErrInvalidPath
	}

	// Clean the path first
	cleaned := filepath.Clean(path)

	// Check for path traversal attempts after cleaning
	// If cleaned path still contains "..", it indicates an attempt to escape
	if strings.Contains(cleaned, "..") {
		return "", ErrPathTraversal
	}

	// If allowedBase is provided, ensure path is within base
	if allowedBase != "" {
		// Clean the base path
		baseCleaned := filepath.Clean(allowedBase)

		// Get absolute paths for reliable comparison
		absPath, err := filepath.Abs(cleaned)
		if err != nil {
			return "", err
		}
		absBase, err := filepath.Abs(baseCleaned)
		if err != nil {
			return "", err
		}

		// Check if path is within base directory
		rel, err := filepath.Rel(absBase, absPath)
		if err != nil {
			return "", ErrPathOutsideBase
		}

		// If relative path starts with "..", path is outside base
		if strings.HasPrefix(rel, "..") {
			return "", ErrPathOutsideBase
		}
	}

	return cleaned, nil
}

// ValidateAndSanitizePath combines validation and sanitization.
// Returns the cleaned, validated path or an error.
func ValidateAndSanitizePath(path, allowedBase string) (string, error) {
	sanitized := SanitizePath(path)
	return ValidatePath(sanitized, allowedBase)
}

// IsSafePath checks if a path is safe without returning the cleaned path.
// Returns true if the path passes validation, false otherwise.
func IsSafePath(path, allowedBase string) bool {
	_, err := ValidatePath(path, allowedBase)
	return err == nil
}

// SafeJoinPath safely joins path elements while validating the result.
// Similar to filepath.Join but validates the final path against allowedBase.
func SafeJoinPath(allowedBase string, elems ...string) (string, error) {
	joined := filepath.Join(elems...)
	return ValidatePath(joined, allowedBase)
}

// ExpandHomeDirectory expands ~/ prefix to user's home directory.
// Returns expanded path or original path if expansion fails.
func ExpandHomeDirectory(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// SecureFilePath ensures a file path is safe for writing.
// It expands home directory, validates path, and ensures it's within
// the user's home directory or a temporary directory.
func SecureFilePath(path string, requireWithinHome bool) (string, error) {
	// Expand ~/ prefix
	expanded := ExpandHomeDirectory(path)

	// Determine allowed base
	var allowedBase string
	if requireWithinHome {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		allowedBase = home
	}

	// Validate path
	return ValidatePath(expanded, allowedBase)
}