package wormhole_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2"
)

func TestFactoryShortcuts(t *testing.T) {
	t.Parallel()

	t.Run("QuickOpenAI", func(t *testing.T) {
		t.Parallel()
		w := wormhole.QuickOpenAI("test-api-key")
		require.NotNil(t, w)
	})

	t.Run("QuickAnthropic", func(t *testing.T) {
		t.Parallel()
		w := wormhole.QuickAnthropic("test-api-key")
		require.NotNil(t, w)
	})

	t.Run("QuickGemini", func(t *testing.T) {
		t.Parallel()
		w := wormhole.QuickGemini("test-api-key")
		require.NotNil(t, w)
	})

	t.Run("QuickOllama", func(t *testing.T) {
		t.Parallel()
		w, err := wormhole.QuickOllama("http://localhost:11434")
		require.NoError(t, err)
		require.NotNil(t, w)
	})

	t.Run("QuickLMStudio", func(t *testing.T) {
		t.Parallel()
		w, err := wormhole.QuickLMStudio("http://localhost:1234")
		require.NoError(t, err)
		require.NotNil(t, w)
	})

	t.Run("QuickLocalOpenAI", func(t *testing.T) {
		t.Parallel()
		w, err := wormhole.QuickLocalOpenAI("http://localhost:8080/v1")
		require.NoError(t, err)
		require.NotNil(t, w)
	})

	t.Run("QuickGroq", func(t *testing.T) {
		t.Parallel()
		w := wormhole.QuickGroq("test-api-key")
		require.NotNil(t, w)
	})

	t.Run("QuickMistral", func(t *testing.T) {
		t.Parallel()
		w := wormhole.QuickMistral("test-api-key")
		require.NotNil(t, w)
	})

	t.Run("QuickOpenRouter", func(t *testing.T) {
		t.Parallel()
		w, err := wormhole.QuickOpenRouter("test-api-key")
		require.NoError(t, err)
		require.NotNil(t, w)
	})
}
