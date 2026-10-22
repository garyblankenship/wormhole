package wormhole_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestStreamAndCollectFidelitySnapshots(t *testing.T) {
	t.Parallel()
	stop := types.FinishReasonStop
	firstSource := types.Source{ID: "first", Raw: map[string]any{"title": "original"}}
	secondSource := types.Source{ID: "second"}
	firstCitation := types.Citation{SourceID: "first", OutputRange: &types.TextRange{End: 2}}
	secondCitation := types.Citation{SourceID: "second"}
	warning := types.ResponseWarning{Code: types.WarningUnsupportedNormalization}
	metadata := &types.ResponseMetadata{RequestID: "fixture", Headers: map[string][]string{"Request-Id": {"original"}}}
	client := collectFixture(t, []types.TextChunk{
		{Text: "hi", ResponseMetadata: metadata, Metadata: map[string]any{"native": map[string]any{"value": "original"}, "latest": "before"},
			Sources: []types.Source{firstSource}, Citations: []types.Citation{firstCitation}, Warnings: []types.ResponseWarning{warning},
			Usage: &types.Usage{CacheWriteTokens: 4, CacheWriteTokensByTTL: map[string]int{"5m": 4}}},
		{FinishReason: &stop, Metadata: map[string]any{"latest": "after"},
			Sources: []types.Source{firstSource, secondSource}, Citations: []types.Citation{firstCitation, secondCitation}, Warnings: []types.ResponseWarning{warning},
			Usage: &types.Usage{CacheWriteTokens: 6, CacheWriteTokensByTTL: map[string]int{"1h": 2}}},
		{Usage: &types.Usage{CacheWriteTokensByTTL: map[string]int{"5m": 3}}},
	})
	stream, result, err := client.Text().Model("fixture").Prompt("hello").StreamAndCollect(t.Context())
	require.NoError(t, err)
	for chunk := range stream {
		if chunk.ResponseMetadata != nil {
			chunk.ResponseMetadata.Headers["Request-Id"][0] = "consumer mutation"
		}
		if chunk.Metadata["native"] != nil {
			chunk.Metadata["native"].(map[string]any)["value"] = "consumer mutation"
		}
		if len(chunk.Sources) > 0 {
			chunk.Sources[0].Raw["title"] = "consumer mutation"
			chunk.Citations[0].OutputRange.End = 99
			chunk.Warnings[0].Code = "consumer mutation"
		}
		if chunk.Usage != nil {
			chunk.Usage.CacheWriteTokensByTTL["5m"] = 99
		}
	}
	response, err := result()
	require.NoError(t, err)
	require.Equal(t, "hi", response.Text)
	require.Equal(t, stop, response.FinishReason)
	require.Equal(t, metadata, response.ResponseMetadata)
	require.Equal(t, map[string]any{"native": map[string]any{"value": "original"}, "latest": "after"}, response.Metadata)
	require.Equal(t, []types.Source{firstSource, secondSource}, response.Sources)
	require.Equal(t, []types.Citation{firstCitation, secondCitation}, response.Citations)
	require.Equal(t, []types.ResponseWarning{warning}, response.Warnings)
	require.Equal(t, 6, response.Usage.CacheWriteTokens)
	require.Equal(t, map[string]int{"5m": 4, "1h": 2}, response.Usage.CacheWriteTokensByTTL)
	response.ResponseMetadata.Headers["Request-Id"][0] = "result mutation"
	response.Metadata["native"].(map[string]any)["value"] = "result mutation"
	response.Sources[0].Raw["title"] = "result mutation"
	response.Citations[0].OutputRange.End = 99
	response.Warnings[0].Code = "result mutation"
	response.Usage.CacheWriteTokensByTTL["5m"] = 99
	again, err := result()
	require.NoError(t, err)
	require.Equal(t, metadata, again.ResponseMetadata)
	require.Equal(t, "original", again.Metadata["native"].(map[string]any)["value"])
	require.Equal(t, []types.Source{firstSource, secondSource}, again.Sources)
	require.Equal(t, []types.Citation{firstCitation, secondCitation}, again.Citations)
	require.Equal(t, []types.ResponseWarning{warning}, again.Warnings)
	require.Equal(t, 4, again.Usage.CacheWriteTokensByTTL["5m"])
}
