package wormhole

import (
	"context"

	"github.com/garyblankenship/wormhole/v3/middleware"
	"github.com/garyblankenship/wormhole/v3/types"
)

func contextWithProviderOperation(ctx context.Context, provider types.Provider, operation, baseURLOverride string) context.Context {
	if provider == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, middleware.CtxKeyProvider, provider.Name())
	ctx = context.WithValue(ctx, middleware.CtxKeyBaseURLOverride, baseURLOverride)
	return context.WithValue(ctx, middleware.CtxKeyMethod, operation)
}
