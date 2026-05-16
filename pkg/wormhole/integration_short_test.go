package wormhole_test

import "testing"

func skipIntegrationInShortMode(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
}
