package fetchers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestConfiguredFetcherConfigCopiesAndCanonicalizesHeaders(t *testing.T) {
	headers := map[string]string{"x-custom": "first", "X-Trace": "trace"}
	configured := newConfiguredFetcherConfig("https://default.example/v1", types.ProviderConfig{
		APIKey:  "primary-key",
		Headers: headers,
	})
	headers["x-custom"] = "changed"

	assert.Equal(t, "https://default.example/v1", configured.baseURL)
	assert.Equal(t, "primary-key", configured.apiKey)
	assert.Equal(t, "first", configured.headers["X-Custom"])
	assert.Equal(t, "trace", configured.headers["X-Trace"])
}

func TestConfiguredAccountDiscriminatorIsStableAndScoped(t *testing.T) {
	base := types.ProviderConfig{
		APIKey:  "secret-key",
		BaseURL: "https://models.example/v1/",
		Headers: map[string]string{"x-tenant": "one", "X-Trace": "yes"},
	}

	stable := NewOpenAIFetcherWithConfig(base).AccountDiscriminator()
	reordered := NewOpenAIFetcherWithConfig(types.ProviderConfig{
		APIKey:  "secret-key",
		BaseURL: "https://models.example/v1",
		Headers: map[string]string{"X-Trace": "yes", "X-Tenant": "one"},
	}).AccountDiscriminator()
	require.Len(t, stable, 64)
	assert.Equal(t, stable, reordered)
	assert.NotContains(t, stable, "secret-key")
	assert.NotContains(t, stable, "models.example")

	for _, config := range []types.ProviderConfig{
		{APIKey: "other-key", BaseURL: base.BaseURL, Headers: base.Headers},
		{APIKey: base.APIKey, BaseURL: "https://other.example/v1", Headers: base.Headers},
		{APIKey: base.APIKey, BaseURL: base.BaseURL, Headers: map[string]string{"X-Tenant": "two", "X-Trace": "yes"}},
		{APIKey: base.APIKey, BaseURL: base.BaseURL, Headers: base.Headers, NoAuth: true},
	} {
		assert.NotEqual(t, stable, NewOpenAIFetcherWithConfig(config).AccountDiscriminator())
	}
}

func TestLegacyFetcherAccountDiscriminatorsRemainUnchanged(t *testing.T) {
	assert.Equal(t, accountKeyDiscriminator("legacy-key"), NewOpenAIFetcher("legacy-key").AccountDiscriminator())
	assert.Equal(t, accountKeyDiscriminator("legacy-key"), NewAnthropicFetcher("legacy-key").AccountDiscriminator())
	assert.Empty(t, NewOpenRouterFetcher().AccountDiscriminator())
}
