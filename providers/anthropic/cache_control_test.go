package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v2/providers"
	"github.com/garyblankenship/wormhole/v2/types"
)

type countingHTTPClient struct {
	requests atomic.Int32
}

func (c *countingHTTPClient) Do(*http.Request) (*http.Response, error) {
	c.requests.Add(1)
	return nil, errors.New("unexpected HTTP request")
}

func TestAnthropicToolCacheControlWireForms(t *testing.T) {
	t.Parallel()

	provider := New(types.NewProviderConfig("key"))
	tests := []struct {
		name         string
		cacheControl *types.CacheControl
		want         string
	}{
		{name: "omitted", want: ""},
		{
			name:         "default ttl",
			cacheControl: &types.CacheControl{Type: types.CacheControlTypeEphemeral, TTL: types.CacheTTLDefault},
			want:         `{"type":"ephemeral"}`,
		},
		{
			name:         "one hour",
			cacheControl: &types.CacheControl{Type: types.CacheControlTypeEphemeral, TTL: types.CacheTTL1Hour},
			want:         `{"type":"ephemeral","ttl":"1h"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tools, err := provider.transformTools([]types.Tool{{
				Name:         "lookup",
				Description:  "Look something up",
				InputSchema:  map[string]any{"type": "object"},
				CacheControl: test.cacheControl,
			}})
			require.NoError(t, err)
			require.Len(t, tools, 1)

			got, present := tools[0]["cache_control"]
			if test.want == "" {
				assert.False(t, present)
				return
			}
			require.True(t, present)
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			assert.Equal(t, test.want, string(encoded))
		})
	}
}

func TestAnthropicToolCacheControlPreservesPlacementAndOrder(t *testing.T) {
	t.Parallel()

	provider := New(types.NewProviderConfig("key"))
	tools, err := provider.transformTools([]types.Tool{
		{Name: "first", InputSchema: map[string]any{"type": "object"}},
		{
			Name:         "second",
			InputSchema:  map[string]any{"type": "object"},
			CacheControl: &types.CacheControl{Type: types.CacheControlTypeEphemeral},
		},
		{Name: "third", InputSchema: map[string]any{"type": "object"}},
	})
	require.NoError(t, err)
	require.Len(t, tools, 3)
	assert.Equal(t, []any{"first", "second", "third"}, []any{tools[0]["name"], tools[1]["name"], tools[2]["name"]})
	assert.NotContains(t, tools[0], "cache_control")
	assert.Contains(t, tools[1], "cache_control")
	assert.NotContains(t, tools[2], "cache_control")
}

func TestAnthropicToolCacheControlValidation(t *testing.T) {
	t.Parallel()

	provider := New(types.NewProviderConfig("key"))
	tests := []struct {
		name         string
		cacheControl types.CacheControl
		wantField    string
	}{
		{name: "empty type", cacheControl: types.CacheControl{}, wantField: "tools[1].cache_control.type"},
		{
			name:         "unsupported type",
			cacheControl: types.CacheControl{Type: "persistent"},
			wantField:    "tools[1].cache_control.type",
		},
		{
			name:         "type validated before ttl",
			cacheControl: types.CacheControl{Type: "persistent", TTL: "2h"},
			wantField:    "tools[1].cache_control.type",
		},
		{
			name:         "unsupported ttl",
			cacheControl: types.CacheControl{Type: types.CacheControlTypeEphemeral, TTL: "2h"},
			wantField:    "tools[1].cache_control.ttl",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := provider.transformTools([]types.Tool{
				{Name: "valid", InputSchema: map[string]any{"type": "object"}},
				{Name: "invalid", InputSchema: map[string]any{"type": "object"}, CacheControl: &test.cacheControl},
			})
			require.Error(t, err)
			validationErr, ok := types.AsValidationError(err)
			require.True(t, ok)
			assert.Equal(t, test.wantField, validationErr.Field)
			assert.False(t, types.IsRetryableError(err))
		})
	}
}

func TestAnthropicRejectsInvalidToolCacheControlBeforeRequest(t *testing.T) {
	t.Parallel()

	config := types.NewProviderConfig("key")
	provider := New(config)
	client := &countingHTTPClient{}
	provider.HTTPClientWrapper = providers.NewHTTPClientWrapper(
		"anthropic",
		config,
		nil,
		&providers.NoAuthStrategy{},
		client,
	)
	request := types.TextRequest{
		BaseRequest: types.BaseRequest{Model: "claude-test"},
		Messages:    []types.Message{types.NewUserMessage("hi")},
		Tools: []types.Tool{{
			Name:         "lookup",
			InputSchema:  map[string]any{"type": "object"},
			CacheControl: &types.CacheControl{Type: types.CacheControlTypeEphemeral, TTL: "2h"},
		}},
	}

	_, err := provider.Text(context.Background(), request)
	require.Error(t, err)
	_, ok := types.AsValidationError(err)
	require.True(t, ok)

	_, err = provider.Stream(context.Background(), request)
	require.Error(t, err)
	_, ok = types.AsValidationError(err)
	require.True(t, ok)
	assert.Zero(t, client.requests.Load())
}

func TestAnthropicStructuredOutputToolHasNoCacheControl(t *testing.T) {
	t.Parallel()

	provider := New(types.NewProviderConfig("key"))
	tool, err := provider.schemaToTool(json.RawMessage(`{"type":"object"}`), "result")
	require.NoError(t, err)
	assert.Nil(t, tool.CacheControl)

	tools, err := provider.transformTools([]types.Tool{*tool})
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.NotContains(t, tools[0], "cache_control")
}
