package wormhole

import (
	"fmt"
)

// parseFloat parses a string to float64, returning nil if parsing fails.
func parseFloat(s string) *float64 {
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
		return &f
	}
	return nil
}

// parseInt parses a string to int, returning nil if parsing fails.
func parseInt(s string) *int {
	var i int
	if _, err := fmt.Sscanf(s, "%d", &i); err == nil {
		return &i
	}
	return nil
}
