package wormhole

import (
	"context"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func contextWithProviderOperation(ctx context.Context, provider types.Provider, operation string) context.Context {
	if provider == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, middleware.CtxKeyProvider, provider.Name())
	return context.WithValue(ctx, middleware.CtxKeyMethod, operation)
}
