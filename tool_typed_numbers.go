package wormhole

import (
	"math"
	"strconv"
)

// parseFloat parses a complete finite float, returning nil on invalid input.
func parseFloat(s string) *float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return &f
}

// parseInt parses a complete integer, returning nil on invalid input.
func parseInt(s string) *int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &i
}
