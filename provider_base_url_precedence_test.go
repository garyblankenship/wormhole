package wormhole

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestNativeProviderOptionsPreserveExplicitBaseURL(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"openai", "anthropic", "gemini"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.NotFoundHandler())
			t.Cleanup(srv.Close)
			url := srv.URL
			config := Config{}
			registerProvider(&config, name, "dummy-key", types.ProviderConfig{BaseURL: url})
			if got := config.Providers[name].BaseURL; got != url {
				t.Fatalf("explicit base URL overridden: %q", got)
			}
		})
	}
}
