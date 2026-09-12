package wormhole

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v3/discovery"
)

// ListAvailableModelsWithStatus returns a provider catalog and whether it came
// from stale cache or fallback data.
func (p *Wormhole) ListAvailableModelsWithStatus(ctx context.Context, provider string) (*discovery.ModelsResult, error) {
	if p.discoveryService == nil {
		return nil, fmt.Errorf("model discovery is not enabled")
	}
	return p.discoveryService.GetModelsWithStatus(ctx, provider)
}
