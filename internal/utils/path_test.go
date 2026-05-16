package utils

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePath(t *testing.T) {
	base := t.TempDir()
	inside := filepath.Join(base, "nested", "file.txt")
	outside := filepath.Join(base, "..", "outside.txt")

	tests := []struct {
		name    string
		path    string
		base    string
		wantErr error
	}{
		{name: "valid relative", path: filepath.Join("nested", ".", "file.txt")},
		{name: "empty", path: "", wantErr: ErrInvalidPath},
		{name: "null byte", path: "bad\x00path", wantErr: ErrInvalidPath},
		{name: "traversal", path: filepath.Join("..", "secret"), wantErr: ErrPathTraversal},
		{name: "inside base", path: inside, base: base},
		{name: "outside base", path: outside, base: base, wantErr: ErrPathOutsideBase},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned, err := ValidatePath(tt.path, tt.base)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr), "got %v, want %v", err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, filepath.Clean(tt.path), cleaned)
		})
	}
}

func TestPathHelpers(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "nested"), 0o755))

	assert.Equal(t, filepath.Clean("a/c"), SanitizePath(filepath.Join("a", "b", "..", "c")))
	assert.True(t, IsSafePath(filepath.Join(base, "nested"), base))
	assert.False(t, IsSafePath(filepath.Join(base, "..", "elsewhere"), base))

	joined, err := SafeJoinPath(base, base, "nested", "file.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "nested", "file.txt"), joined)

	_, err = ValidateAndSanitizePath(filepath.Join("..", "secret"), "")
	require.ErrorIs(t, err, ErrPathTraversal)
}

func TestExpandHomeDirectoryAndSecureFilePath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	expanded := ExpandHomeDirectory("~/wormhole-test.txt")
	assert.Equal(t, filepath.Join(home, "wormhole-test.txt"), expanded)
	assert.Equal(t, "/tmp/wormhole-test.txt", ExpandHomeDirectory("/tmp/wormhole-test.txt"))

	path, err := SecureFilePath("~/wormhole-test.txt", true)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(path, home))

	_, err = SecureFilePath(filepath.Join(home, "..", "outside.txt"), true)
	require.Error(t, err)
}
