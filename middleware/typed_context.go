package middleware

import "context"

func providerIdentity(ctx context.Context) string {
	provider, _ := ctx.Value(CtxKeyProvider).(string)
	if provider == "" {
		return "default"
	}
	return provider
}
