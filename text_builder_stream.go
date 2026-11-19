package wormhole

import (
	"context"
	"fmt"
	"sync"

	"github.com/garyblankenship/wormhole/v3/types"
)

const streamAttemptsErrorMessage = "all stream attempts failed before emitting a chunk"

// streamAttemptsError retains each failed stream attempt for errors.Is and
// errors.As without exposing provider-controlled error text in Error.
type streamAttemptsError struct {
	attempts []error
}

func newStreamAttemptsError(attempts []error) *streamAttemptsError {
	return &streamAttemptsError{attempts: append([]error(nil), attempts...)}
}

func (e *streamAttemptsError) Error() string {
	return streamAttemptsErrorMessage
}

// Unwrap returns a copy so callers cannot modify the aggregate's attempt order.
func (e *streamAttemptsError) Unwrap() []error {
	if e == nil {
		return nil
	}
	return append([]error(nil), e.attempts...)
}

// StreamAttemptErrors returns failed routes in attempt order. Consumers presenting
// one outcome can select the final attempt while errors.Is/As retain all causes.
func (e *streamAttemptsError) StreamAttemptErrors() []error {
	return e.Unwrap()
}

// Stream executes the request and returns a streaming response
func (b *TextRequestBuilder) Stream(ctx context.Context) (<-chan types.StreamChunk, error) {
	baseRequest := cloneTextRequest(b.request)
	prepareTextExecutionRequest(baseRequest)

	if len(baseRequest.Messages) == 0 {
		return nil, types.ErrInvalidRequest.WithDetails("no messages provided")
	}
	if baseRequest.Model == "" {
		return nil, types.ErrInvalidRequest.WithDetails("no model specified")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	modelsToTry := make([]string, 0, 1+len(b.fallbackModels))
	modelsToTry = append(modelsToTry, baseRequest.Model)
	modelsToTry = append(modelsToTry, b.fallbackModels...)
	wormhole := b.getWormhole()
	primaryValidated := len(b.fallbackModels) == 0 && len(b.providerFallbacks) == 0
	if primaryValidated {
		if err := wormhole.validateModelAttempt(b.getProvider(), baseRequest.Model, textModelCapabilities, textRequiredCapabilities(baseRequest, false, true)); err != nil {
			providerName, _ := wormhole.resolveProviderName(b.getProvider())
			event := AttemptEvent{Operation: "text.stream", Phase: AttemptStarted, Provider: providerName, Model: baseRequest.Model, Attempt: 1, Stream: true}
			wormhole.emitAttempt(ctx, event)
			event.Phase, event.Error = AttemptError, err
			wormhole.emitAttempt(ctx, event)
			return nil, err
		}
	}

	if !wormhole.trackRequest() {
		return nil, fmt.Errorf("client is shutting down")
	}

	provider, release, err := b.getProviderWithBaseURLContext(ctx)
	if err != nil {
		b.getWormhole().untrackRequest()
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		release()
		wormhole.untrackRequest()
		return nil, err
	}

	stream := make(chan types.StreamChunk)
	providerFallbacks := append([]TextRoute(nil), b.providerFallbacks...)
	go b.streamWithFallback(ctx, provider, release, b.getProvider(), baseRequest, modelsToTry, providerFallbacks, primaryValidated, stream)
	return stream, nil
}

func (b *TextRequestBuilder) streamWithFallback(ctx context.Context, provider types.Provider, release func(), primaryProviderName string, baseRequest *types.TextRequest, modelsToTry []string, providerFallbacks []TextRoute, primaryValidated bool, out chan<- types.StreamChunk) {
	defer close(out)
	defer b.getWormhole().untrackRequest()
	release = sync.OnceFunc(release)
	defer release()

	wormhole := b.getWormhole()
	routes := make([]TextRoute, 0, len(modelsToTry)+len(providerFallbacks))
	for _, model := range modelsToTry {
		routes = append(routes, TextRoute{Provider: primaryProviderName, Model: model})
	}
	routes = append(routes, providerFallbacks...)

	var attemptErrors []error
	var lastErr error
	var lastStreamEvent StreamEvent
	for index, route := range routes {
		primary := index < len(modelsToTry)
		traceProvider := route.Provider
		if primary {
			traceProvider = provider.Name()
		}
		attempt := index + 1
		if ctx.Err() != nil {
			if lastStreamEvent.Attempt == 0 {
				lastStreamEvent = StreamEvent{Provider: traceProvider, Model: route.Model, Attempt: attempt}
			}
			lastStreamEvent.Type = StreamError
			lastStreamEvent.Error = ctx.Err()
			wormhole.emitStreamEvent(ctx, lastStreamEvent)
			return
		}
		if index == len(modelsToTry) {
			// Keep the primary lease across model fallbacks, then relinquish it
			// before acquiring any provider fallback.
			release()
			if ctx.Err() != nil {
				if lastStreamEvent.Attempt == 0 {
					lastStreamEvent = StreamEvent{Provider: traceProvider, Model: route.Model, Attempt: attempt}
				}
				lastStreamEvent.Type = StreamError
				lastStreamEvent.Error = ctx.Err()
				wormhole.emitStreamEvent(ctx, lastStreamEvent)
				return
			}
		}
		event := AttemptEvent{
			Operation: "text.stream",
			Phase:     AttemptStarted,
			Provider:  traceProvider,
			Model:     route.Model,
			Attempt:   attempt,
			Fallback:  index > 0,
			Stream:    true,
		}
		wormhole.emitAttempt(ctx, event)
		streamEvent := StreamEvent{Provider: event.Provider, Model: event.Model, Attempt: event.Attempt}
		lastStreamEvent = streamEvent

		result := func() streamAttemptResult {
			if err := ctx.Err(); err != nil {
				return streamAttemptResult{err: err}
			}
			request := cloneTextRequest(baseRequest)
			request.Model = route.Model
			// A single route was validated synchronously so Stream can return
			// capability errors directly. Other routes are validated when tried.
			if !primaryValidated || index > 0 {
				if err := wormhole.validateModelAttempt(route.Provider, route.Model, textModelCapabilities, textRequiredCapabilities(request, false, true)); err != nil {
					return streamAttemptResult{err: err}
				}
			}
			attemptProvider := provider
			if !primary {
				leased, releaseFallback, err := wormhole.leaseProviderContext(ctx, route.Provider)
				if err != nil {
					return streamAttemptResult{err: err}
				}
				defer releaseFallback()
				attemptProvider = leased
			}
			// Factories can block without a context. Cancellation while acquiring
			// a lease must release it before any upstream request starts.
			if err := ctx.Err(); err != nil {
				return streamAttemptResult{err: err}
			}
			attemptCtx, cancelAttempt := context.WithCancel(ctx)
			defer cancelAttempt()
			baseURL := ""
			if primary {
				baseURL = b.getBaseURL()
			}
			stream, err := b.openStream(attemptCtx, cancelAttempt, attemptProvider, request, baseURL)
			if err != nil {
				return streamAttemptResult{err: err}
			}
			streamEvent.Type = StreamStarted
			wormhole.emitStreamEvent(ctx, streamEvent)
			return forwardStreamWithFirstChunkSafety(ctx, cancelAttempt, out, stream)
		}()

		if result.err != nil {
			lastErr = result.err
			attemptErrors = append(attemptErrors, result.err)
			event.Phase = AttemptError
			event.Error = result.err
		} else {
			event.Phase = AttemptSuccess
		}
		wormhole.emitAttempt(ctx, event)
		if result.canFallback(ctx) {
			continue
		}
		streamErr := result.err
		if streamErr == nil {
			streamErr = ctx.Err()
		}
		if streamErr != nil {
			streamEvent.Type = StreamError
			streamEvent.Error = streamErr
		} else {
			streamEvent.Type = StreamEnded
		}
		wormhole.emitStreamEvent(ctx, streamEvent)
		return
	}

	release()

	if ctx.Err() != nil {
		lastStreamEvent.Type = StreamError
		lastStreamEvent.Error = ctx.Err()
		wormhole.emitStreamEvent(ctx, lastStreamEvent)
		return
	}
	if len(modelsToTry)+len(providerFallbacks) == 1 && lastErr != nil {
		sendStreamChunk(ctx, out, types.StreamChunk{Error: lastErr})
		lastStreamEvent.Type = StreamError
		lastStreamEvent.Error = lastErr
		wormhole.emitStreamEvent(ctx, lastStreamEvent)
		return
	}
	aggregateErr := newStreamAttemptsError(attemptErrors)
	sendStreamChunk(ctx, out, types.StreamChunk{Error: aggregateErr})
	lastStreamEvent.Type = StreamError
	lastStreamEvent.Error = aggregateErr
	wormhole.emitStreamEvent(ctx, lastStreamEvent)
}
