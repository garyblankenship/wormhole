package middleware

// contextKey is an unexported type for context keys in the middleware package.
// Using a distinct type prevents collisions with keys defined in other packages.
type contextKey string

// Context keys used by middleware to extract request metadata.
const (
	// CtxKeyProvider identifies the LLM provider (e.g. "openai", "anthropic").
	CtxKeyProvider contextKey = "provider"

	// CtxKeyModel identifies the model being used.
	CtxKeyModel contextKey = "model"

	// CtxKeyMethod identifies the request method (e.g. "text", "stream").
	CtxKeyMethod contextKey = "method"

	// CtxKeyWormholeProvider is an alternative provider key used by the typed
	// enhanced metrics middleware.
	CtxKeyWormholeProvider contextKey = "wormhole_provider"
)
