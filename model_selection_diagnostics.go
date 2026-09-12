package wormhole

import (
	"context"

	"github.com/garyblankenship/wormhole/v3/types"
)

// ModelDiscoveryDiagnostic describes one provider catalog read attempted during
// model selection. Err reports only the GetModelsWithStatus error observed by
// selection; background stale-cache refresh errors are intentionally absent.
type ModelDiscoveryDiagnostic struct {
	Provider string
	Stale    bool
	Err      error
}

// ModelSelectionResult contains matching models and the discovery reads used to
// select them.
type ModelSelectionResult struct {
	Models      []*types.ModelInfo
	Diagnostics []ModelDiscoveryDiagnostic
}

// SelectModelsWithDiagnostics returns models matching query and diagnostics for
// every provider catalog read attempted during selection.
func (p *Wormhole) SelectModelsWithDiagnostics(ctx context.Context, query ModelQuery) (*ModelSelectionResult, error) {
	return p.selectModels(ctx, query)
}
