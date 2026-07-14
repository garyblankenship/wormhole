package wormhole

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRerankBuilderValidate(t *testing.T) {
	t.Parallel()
	client := New()

	// Missing all required fields.
	assert.Error(t, client.Rerank().Validate())
	// Missing documents.
	assert.Error(t, client.Rerank().Model("cohere/rerank-v3.5").Query("q").Validate())
	// Complete request.
	assert.NoError(t, client.Rerank().Model("cohere/rerank-v3.5").Query("q").Documents("a", "b").Validate())
}
