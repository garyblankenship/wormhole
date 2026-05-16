package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecureRandomInt(t *testing.T) {
	n, err := SecureRandomInt(10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(0))
	assert.Less(t, n, int64(10))

	n, err = SecureRandomInt(0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestSecureRandomFloatRange(t *testing.T) {
	value, err := SecureRandomFloatRange(2.5, 5.5)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, value, 2.5)
	assert.Less(t, value, 5.5)

	value, err = SecureRandomFloatRange(5.5, 2.5)
	require.NoError(t, err)
	assert.Equal(t, 5.5, value)
}

func TestSecureRandomIntRange(t *testing.T) {
	n, err := SecureRandomIntRange(10, 20)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(10))
	assert.Less(t, n, int64(20))

	n, err = SecureRandomIntRange(20, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(20), n)
}

func TestNewCryptoSeededRand(t *testing.T) {
	rng, err := NewCryptoSeededRand()
	require.NoError(t, err)
	require.NotNil(t, rng)
	assert.GreaterOrEqual(t, rng.Int63(), int64(0))
}
