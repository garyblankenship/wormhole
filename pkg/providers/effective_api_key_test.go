package providers

import (
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestHTTPClientWrapperUsesEffectiveAPIKey(t *testing.T) {
	wrapper := NewHTTPClientWrapper(
		"test",
		types.ProviderConfig{APIKeys: []string{"first", "second"}},
		nil,
		&BearerAuthStrategy{},
		nil,
	)
	t.Cleanup(func() { _ = wrapper.Close() })

	if wrapper.Config.APIKey != "first" {
		t.Fatalf("wrapper APIKey = %q, want first pooled key", wrapper.Config.APIKey)
	}
}
