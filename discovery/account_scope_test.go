package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

type stubFetcher struct {
	name string
}

func (f *stubFetcher) Name() string { return f.name }

func (f *stubFetcher) FetchModels(ctx context.Context) ([]*types.ModelInfo, error) {
	return nil, nil
}

type stubAccountFetcher struct {
	stubFetcher
	discriminator string
}

func (f *stubAccountFetcher) AccountDiscriminator() string { return f.discriminator }

func TestAccountCacheKeyScopesByCredential(t *testing.T) {
	t.Parallel()

	accountA := &stubAccountFetcher{stubFetcher: stubFetcher{name: "openai"}, discriminator: "aaaa1111"}
	accountB := &stubAccountFetcher{stubFetcher: stubFetcher{name: "openai"}, discriminator: "bbbb2222"}

	keyA := accountCacheKey("openai", accountA)
	keyB := accountCacheKey("openai", accountB)

	if keyA == keyB {
		t.Fatalf("expected distinct cache keys for distinct accounts, got %q for both", keyA)
	}
	if keyA != "openai__aaaa1111" {
		t.Fatalf("unexpected cache key: %q", keyA)
	}
}

func TestAccountCacheKeyFallsBackWithoutDiscriminator(t *testing.T) {
	t.Parallel()

	unscoped := &stubFetcher{name: "ollama"}
	if key := accountCacheKey("ollama", unscoped); key != "ollama" {
		t.Fatalf("expected plain provider name for unscoped fetcher, got %q", key)
	}

	emptyDiscriminator := &stubAccountFetcher{stubFetcher: stubFetcher{name: "openrouter"}, discriminator: ""}
	if key := accountCacheKey("openrouter", emptyDiscriminator); key != "openrouter" {
		t.Fatalf("expected plain provider name for empty discriminator, got %q", key)
	}
}

func TestDiscoveryServiceOfflineScopedProviderUsesBaseFallback(t *testing.T) {
	t.Parallel()

	service := NewDiscoveryService(DiscoveryConfig{
		CacheTTL:        time.Hour,
		EnableFileCache: false,
		OfflineMode:     true,
	}, &stubAccountFetcher{
		stubFetcher:   stubFetcher{name: "openai"},
		discriminator: "acct1234",
	})

	models, err := service.GetModels(context.Background(), "openai")
	if err != nil {
		t.Fatalf("expected fallback models for account-scoped provider, got %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected fallback models for account-scoped provider")
	}
}
