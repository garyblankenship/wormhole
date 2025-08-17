package utils

import (
	"encoding/json"
	"fmt"
)

// LenientUnmarshal attempts to unmarshal JSON with fallback handling for common escape sequence issues
// that can occur with AI model responses containing regex patterns or code examples.
func LenientUnmarshal(data []byte, v interface{}) error {
	// First, try standard JSON unmarshaling
	if err := json.Unmarshal(data, v); err != nil {
		// For now, if standard unmarshaling fails, we return the error
		// This ensures we don't break existing functionality
		// In the future, we can add specific fixes for known AI model issues
		return err
	}

	return nil
}

// UnmarshalAnthropicToolArgs is a specialized function for unmarshaling Anthropic tool arguments
// that may contain regex patterns or escaped strings
func UnmarshalAnthropicToolArgs(args string, v interface{}) error {
	if args == "" {
		return fmt.Errorf("empty tool arguments")
	}

	// Try standard JSON unmarshaling first
	if err := json.Unmarshal([]byte(args), v); err != nil {
		// If it fails, add context about the tool arguments parsing failure
		return fmt.Errorf("failed to parse Anthropic tool arguments (may contain regex or escaped patterns): %w", err)
	}

	return nil
}
