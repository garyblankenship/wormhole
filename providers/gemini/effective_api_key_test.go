package gemini

import (
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
)

func TestNewUsesEffectiveAPIKey(t *testing.T) {
	provider := New("", types.ProviderConfig{APIKeys: []string{"first", "second"}})
	t.Cleanup(func() { _ = provider.Close() })

	if provider.Config.APIKey != "first" {
		t.Fatalf("Gemini APIKey = %q, want first pooled key", provider.Config.APIKey)
	}
}
