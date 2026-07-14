package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandPathValidationAndFallback(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"", "bad\x00path", filepath.Join("..", "secret")} {
		got, err := expandPath(path)
		require.NoError(t, err)
		assert.Equal(t, "wormhole-cache.json", got)
	}

	got, err := expandPath(filepath.Join("cache", "models.json"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("cache", "models.json"), got)
}

func TestExpandPathHomePrefix(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	got, err := expandPath("~/.wormhole/models.json")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".wormhole", "models.json"), got)
}
