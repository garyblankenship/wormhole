package wormhole

import (
	"context"
	"sync"

	"github.com/garyblankenship/wormhole/v2/types"
)

// Generate executes the request and returns a response
func (b *TextRequestBuilder) Generate(ctx context.Context) (*types.TextResponse, error) {
	baseRequest := cloneTextRequest(b.request)
	prepareTextExecutionRequest(baseRequest)

	if len(baseRequest.Messages) == 0 {
		return nil, types.ErrInvalidRequest.WithDetails("no messages provided")
	}
	if baseRequest.Model == "" {
		return nil, types.ErrInvalidRequest.WithDetails("no model specified")
	}

	// Build list of models to try (primary + fallbacks)
	modelsToTry := make([]string, 0, 1+len(b.fallbackModels))
	modelsToTry = append(modelsToTry, baseRequest.Model)
	modelsToTry = append(modelsToTry, b.fallbackModels...)
	idempotencyRequest := textIdempotencyRequest{
		Request:           baseRequest,
		FallbackModels:    append([]string(nil), b.fallbackModels...),
		ProviderFallbacks: append([]TextRoute(nil), b.providerFallbacks...),
	}

	return executeTrackedRequest(ctx, b.getWormhole(), b.idempotencyScope("text.generate"), idempotencyRequest, func(ctx context.Context) (*types.TextResponse, error) {
		provider, release, err := b.getProviderWithBaseURL()
		if err != nil {
			return nil, err
		}
		release = sync.OnceFunc(release)
		defer release()

		var lastErr error
		wormhole := b.getWormhole()
		for attempt, model := range modelsToTry {
			request := cloneTextRequest(baseRequest)
			request.Model = model
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.generate",
				Phase:     AttemptStarted,
				Provider:  provider.Name(),
				Model:     model,
				Attempt:   attempt + 1,
				Fallback:  attempt > 0,
			})

			resp, err := b.executeGenerate(ctx, provider, request)
			if err == nil {
				wormhole.emitAttempt(ctx, AttemptEvent{
					Operation: "text.generate",
					Phase:     AttemptSuccess,
					Provider:  provider.Name(),
					Model:     model,
					Attempt:   attempt + 1,
					Fallback:  attempt > 0,
				})
				return resp, nil
			}
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.generate",
				Phase:     AttemptError,
				Provider:  provider.Name(),
				Model:     model,
				Attempt:   attempt + 1,
				Fallback:  attempt > 0,
				Error:     err,
			})
			lastErr = err
		}
		release()
		if ctx.Err() != nil {
			return nil, lastErr
		}

		for routeIndex, route := range b.providerFallbacks {
			attempt := len(modelsToTry) + routeIndex + 1
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.generate",
				Phase:     AttemptStarted,
				Provider:  route.Provider,
				Model:     route.Model,
				Attempt:   attempt,
				Fallback:  true,
			})

			response, err := func() (*types.TextResponse, error) {
				provider, release, err := wormhole.leaseProvider(route.Provider)
				if err != nil {
					return nil, err
				}
				defer release()
				request := cloneTextRequest(baseRequest)
				request.Model = route.Model
				return b.executeGenerate(ctx, provider, request)
			}()
			if err == nil {
				wormhole.emitAttempt(ctx, AttemptEvent{
					Operation: "text.generate",
					Phase:     AttemptSuccess,
					Provider:  route.Provider,
					Model:     route.Model,
					Attempt:   attempt,
					Fallback:  true,
				})
				return response, nil
			}

			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.generate",
				Phase:     AttemptError,
				Provider:  route.Provider,
				Model:     route.Model,
				Attempt:   attempt,
				Fallback:  true,
				Error:     err,
			})
			lastErr = err
			if ctx.Err() != nil {
				return nil, lastErr
			}
		}

		return nil, lastErr
	})
}

// executeGenerate performs the actual generation with the current request settings
func (b *TextRequestBuilder) executeGenerate(ctx context.Context, provider types.Provider, request *types.TextRequest) (*types.TextResponse, error) {
	// Check if we should enable automatic tool execution
	wormhole := b.getWormhole()
	ctx = contextWithProviderOperation(ctx, provider, "text")
	shouldAutoExecuteTools := b.shouldAutoExecuteTools(wormhole)

	// If auto-execution is enabled, use the tool executor
	if shouldAutoExecuteTools {
		executor := NewToolExecutor(wormhole.toolRegistry)
		maxIterations := b.maxToolIterations
		if maxIterations == 0 {
			maxIterations = 10 // Default
		}

		// ExecuteWithTools will handle middleware internally by calling provider.Text
		// which goes through the middleware chain
		return executor.ExecuteWithTools(ctx, *request, provider, maxIterations)
	}

	// Standard execution without automatic tool handling

	// Apply type-safe middleware chain if configured
	if wormhole.providerMiddleware != nil {
		handler := wormhole.providerMiddleware.ApplyText(provider.Text)
		return handler(ctx, *request)
	}

	// No middleware configured, use provider directly
	return provider.Text(ctx, *request)
}

// shouldAutoExecuteTools determines if automatic tool execution should be enabled
func (b *TextRequestBuilder) shouldAutoExecuteTools(wormhole *Wormhole) bool {
	// Explicit WithToolsEnabled/WithToolsDisabled call always wins.
	if b.toolExecutionOverride != nil {
		return *b.toolExecutionOverride
	}

	// Unset: auto-enable if tools are registered on the client AND no tools
	// were explicitly set on the request (use registry tools).
	if wormhole.toolRegistry.Count() > 0 && len(b.request.Tools) == 0 {
		return true
	}
	return false
}
