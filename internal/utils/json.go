package utils

import (
	"encoding/json"
	"fmt"
	"strings"
)

// LenientUnmarshal attempts to unmarshal JSON with fallback handling for common escape sequence issues
// that can occur with AI model responses containing regex patterns or code examples.
func LenientUnmarshal(data []byte, v any) error {
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
func UnmarshalAnthropicToolArgs(args string, v any) error {
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

// ExtractJSONFromMarkdown removes markdown code blocks from JSON responses.
// This handles common patterns where LLMs return JSON wrapped in ```json or ``` blocks.
// If no code blocks are present, returns the original content unchanged.
func ExtractJSONFromMarkdown(content string) string {
	if !strings.Contains(content, "```") {
		return content
	}

	// Try to extract from ```json code blocks first
	if strings.Contains(content, "```json") {
		start := strings.Index(content, "```json") + 7
		end := strings.LastIndex(content, "```")
		if start < end {
			return strings.TrimSpace(content[start:end])
		}
	}

	// Try generic code blocks
	if strings.Contains(content, "```") {
		start := strings.Index(content, "```") + 3
		end := strings.LastIndex(content, "```")
		if start < end {
			cleaned := strings.TrimSpace(content[start:end])
			// Only return cleaned version if it looks like JSON
			if (strings.HasPrefix(cleaned, "{") && strings.HasSuffix(cleaned, "}")) ||
				(strings.HasPrefix(cleaned, "[") && strings.HasSuffix(cleaned, "]")) {
				return cleaned
			}
		}
	}

	return content
}
