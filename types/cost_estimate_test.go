package types

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageCostEstimateAccounting(t *testing.T) {
	t.Parallel()
	read, write := 0.2, 2.0
	cost := &ModelCost{InputTokens: 1, OutputTokens: 3, Currency: "USD"}
	for _, accounting := range []PromptTokenAccounting{PromptTokensInclusiveCache, PromptTokensUncached} {
		t.Run(map[PromptTokenAccounting]string{PromptTokensInclusiveCache: "inclusive", PromptTokensUncached: "uncached"}[accounting], func(t *testing.T) {
			t.Parallel()
			usage := Usage{PromptTokens: 1000, CompletionTokens: 100, CacheReadTokens: 200, CacheWriteTokens: 300, ReasoningTokens: 50, TotalTokens: 9000}
			if accounting == PromptTokensUncached {
				usage.PromptTokens = 500
			}
			result, err := EstimateUsageCost(cost, usage, UsageCostOptions{PromptAccounting: accounting, CacheReadRate: &read, CacheWriteRate: &write})
			require.NoError(t, err)
			require.True(t, result.Known)
			require.Equal(t, "USD", result.Currency)
			require.InDelta(t, 0.5, *result.Input, 1e-12)
			require.InDelta(t, 0.3, *result.Output, 1e-12)
			require.InDelta(t, 0.04, *result.CacheRead, 1e-12)
			require.InDelta(t, 0.6, *result.CacheWrite, 1e-12)
			require.InDelta(t, 1.44, *result.Total, 1e-12)
		})
	}
}

func TestUsageCostEstimateUnknownAndZero(t *testing.T) {
	t.Parallel()
	zero := 0.0
	cases := []struct {
		name        string
		cost        *ModelCost
		usage       Usage
		read, write *float64
		known       bool
	}{
		{"unknown base", nil, Usage{}, nil, nil, false},
		{"priced zero", &ModelCost{}, Usage{PromptTokens: 20}, nil, nil, true},
		{"unused cache rates", &ModelCost{InputTokens: 1}, Usage{}, nil, nil, true},
		{"unknown cache read", &ModelCost{}, Usage{PromptTokens: 5, CacheReadTokens: 5}, nil, nil, false},
		{"unknown cache write", &ModelCost{}, Usage{PromptTokens: 5, CacheWriteTokens: 5}, nil, nil, false},
		{"free cache", &ModelCost{}, Usage{PromptTokens: 5, CacheReadTokens: 3, CacheWriteTokens: 2}, &zero, &zero, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := EstimateUsageCost(tc.cost, tc.usage, UsageCostOptions{PromptAccounting: PromptTokensInclusiveCache, CacheReadRate: tc.read, CacheWriteRate: tc.write})
			require.NoError(t, err)
			require.Equal(t, tc.known, result.Known)
			if tc.known {
				require.NotNil(t, result.Total)
				require.Zero(t, *result.Total)
			} else {
				require.Nil(t, result.Total)
			}
		})
	}
	partial, err := EstimateUsageCost(&ModelCost{InputTokens: 1}, Usage{PromptTokens: 1000, CacheReadTokens: 500}, UsageCostOptions{PromptAccounting: PromptTokensInclusiveCache})
	require.NoError(t, err)
	require.Equal(t, 0.5, *partial.Input)
	require.Nil(t, partial.CacheRead)
	require.Nil(t, partial.Total)
}

func TestUsageCostEstimateValidation(t *testing.T) {
	t.Parallel()
	badRate := math.NaN()
	cases := []struct {
		name    string
		cost    *ModelCost
		usage   Usage
		options UsageCostOptions
	}{
		{name: "unspecified"},
		{name: "unknown accounting", options: UsageCostOptions{PromptAccounting: 99}},
		{name: "negative prompt", usage: Usage{PromptTokens: -1}},
		{name: "negative completion", usage: Usage{CompletionTokens: -1}},
		{name: "negative total", usage: Usage{TotalTokens: -1}},
		{name: "negative read", usage: Usage{CacheReadTokens: -1}},
		{name: "negative write", usage: Usage{CacheWriteTokens: -1}},
		{name: "negative reasoning", usage: Usage{ReasoningTokens: -1}},
		{name: "read overflow", usage: Usage{PromptTokens: 1, CacheReadTokens: 2}},
		{name: "combined cache overflow", usage: Usage{PromptTokens: 2, CacheReadTokens: 2, CacheWriteTokens: 1}},
		{name: "integer overflow", usage: Usage{PromptTokens: math.MaxInt, CacheReadTokens: math.MaxInt, CacheWriteTokens: math.MaxInt}},
		{name: "negative price", cost: &ModelCost{InputTokens: -1}},
		{name: "infinite price", cost: &ModelCost{OutputTokens: math.Inf(1)}},
		{name: "nan cache", options: UsageCostOptions{CacheReadRate: &badRate}},
		{name: "nonfinite charge", cost: &ModelCost{InputTokens: math.MaxFloat64}, usage: Usage{PromptTokens: math.MaxInt}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.name != "unspecified" && tc.name != "unknown accounting" {
				tc.options.PromptAccounting = PromptTokensInclusiveCache
			}
			_, err := EstimateUsageCost(tc.cost, tc.usage, tc.options)
			require.Error(t, err)
			var typed *WormholeError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, ErrorCodeRequest, typed.Code)
		})
	}
}

func TestUsageCostEstimateRegistryAndLegacy(t *testing.T) {
	t.Parallel()
	registry := NewModelRegistry()
	registry.Register(&ModelInfo{ID: "priced", Cost: &ModelCost{InputTokens: 1, OutputTokens: 2, Currency: "credits"}})
	registry.Register(&ModelInfo{ID: "unknown"})
	options := UsageCostOptions{PromptAccounting: PromptTokensUncached}
	priced, err := registry.EstimateUsageCost("priced", Usage{PromptTokens: 1000}, options)
	require.NoError(t, err)
	require.Equal(t, "credits", priced.Currency)
	require.Equal(t, 1.0, *priced.Total)
	unknown, err := registry.EstimateUsageCost("unknown", Usage{}, options)
	require.NoError(t, err)
	require.False(t, unknown.Known)
	_, err = registry.EstimateUsageCost("missing", Usage{}, options)
	require.Error(t, err)
	legacy, err := registry.EstimateCost("unknown", 1000, 1000)
	require.NoError(t, err)
	require.Zero(t, legacy)
	legacy, err = registry.EstimateCost("priced", 1000, 1000)
	require.NoError(t, err)
	require.Equal(t, 3.0, legacy)
}
