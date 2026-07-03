package wormhole

import (
	"context"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func contextWithProvider(ctx context.Context, provider types.Provider) context.Context {
	if provider == nil {
		return ctx
	}
	return context.WithValue(ctx, middleware.CtxKeyProvider, provider.Name())
}
