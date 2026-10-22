package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestCitationUnsupportedLocationRetainsSource(t *testing.T) {
	t.Parallel()
	raw := map[string]any{
		"type": "unknown", "document_index": float64(1), "document_title": "retained",
	}
	data, err := json.Marshal(raw)
	require.NoError(t, err)
	acc := newCitationAccumulator()
	acc.add(data)

	require.Len(t, acc.sources, 1)
	require.Equal(t, raw, acc.sources[0].Raw)
	require.Equal(t, "retained", acc.sources[0].Title)
	require.Empty(t, acc.citations)
	require.Len(t, acc.warnings, 1)
	require.Equal(t, types.WarningUnsupportedNormalization, acc.warnings[0].Code)
}

func TestCitationSourceIdentifierPrecedence(t *testing.T) {
	t.Parallel()
	raw := map[string]any{
		"type":           "web_search_result_location",
		"document_index": float64(1), "encrypted_index": "web-id", "search_result_index": float64(3),
	}
	data, err := json.Marshal(raw)
	require.NoError(t, err)
	acc := newCitationAccumulator()
	acc.add(data)

	require.Len(t, acc.sourceIDs, 1)
	require.Equal(t, "source_1", acc.sourceIDs["search:3"])
	require.Len(t, acc.citations, 1)
	require.Equal(t, "source_1", acc.citations[0].SourceID)
}
