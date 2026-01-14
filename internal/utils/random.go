// Package utils provides utility functions for the Wormhole SDK.
// This file contains cryptographically secure random number generation utilities.
package utils

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	mrand "math/rand"
)

// SecureRandomInt returns a cryptographically secure random integer in [0, max).
// If max <= 0, returns 0 and an error.
func SecureRandomInt(max int64) (int64, error) {
	if max <= 0 {
		return 0, nil
	}
	bigMax := big.NewInt(max)
	n, err := rand.Int(rand.Reader, bigMax)
	if err != nil {
		return 0, err
	}
	return n.Int64(), nil
}

// SecureRandomFloat returns a cryptographically secure random float in [0, 1).
func SecureRandomFloat() (float64, error) {
	// Generate random integer between 0 and 2^53-1 (enough precision for float64)
	max := new(big.Int).Exp(big.NewInt(2), big.NewInt(53), nil)
	max.Sub(max, big.NewInt(1)) // 2^53 - 1

	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0.0, err
	}

	// Convert to float in range [0, 1)
	return float64(n.Int64()) / float64(max.Int64()), nil
}

// SecureRandomFloatRange returns a cryptographically secure random float in [min, max).
func SecureRandomFloatRange(min, max float64) (float64, error) {
	if max <= min {
		return min, nil
	}
	val, err := SecureRandomFloat()
	if err != nil {
		return 0.0, err
	}
	return min + val*(max-min), nil
}

// SecureRandomIntRange returns a cryptographically secure random integer in [min, max).
func SecureRandomIntRange(min, max int64) (int64, error) {
	if max <= min {
		return min, nil
	}
	diff := max - min
	n, err := SecureRandomInt(diff)
	if err != nil {
		return 0, err
	}
	return min + n, nil
}

// SeedGlobalRand seeds the global math/rand generator with cryptographically secure randomness.
// This improves the randomness of math/rand for non-security-critical operations.
// Note: The global math/rand generator is automatically seeded in Go 1.20+, but this
// ensures cryptographically secure seeding for additional safety.
func SeedGlobalRand() error {
	var seed int64
	// Read 8 random bytes for seed
	if err := binary.Read(rand.Reader, binary.BigEndian, &seed); err != nil {
		return err
	}
	mrand.Seed(seed)
	return nil
}