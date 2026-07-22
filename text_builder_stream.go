package wormhole

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/garyblankenship/wormhole/v2/types"
)

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

	modelsToTry := make([]string, 0, 1+len(b.fallbackModels))
	modelsToTry = append(modelsToTry, baseRequest.Model)
	modelsToTry = append(modelsToTry, b.fallbackModels...)
	wormhole := b.getWormhole()
	if len(b.fallbackModels) == 0 && len(b.providerFallbacks) == 0 {
		if err := wormhole.validateModelAttempt(b.getProvider(), baseRequest.Model, textModelCapabilities, textRequiredCapabilities(baseRequest, false, true)); err != nil {
			providerName, _ := wormhole.resolveProviderName(b.getProvider())
			wormhole.emitAttempt(ctx, AttemptEvent{Operation: "text.stream", Phase: AttemptStarted, Provider: providerName, Model: baseRequest.Model, Attempt: 1, Stream: true})
			wormhole.emitAttempt(ctx, AttemptEvent{Operation: "text.stream", Phase: AttemptError, Provider: providerName, Model: baseRequest.Model, Attempt: 1, Stream: true, Error: err})
			return nil, err
		}
	}

	if !wormhole.trackRequest() {
		return nil, fmt.Errorf("client is shutting down")
	}

	provider, release, err := b.getProviderWithBaseURL()
	if err != nil {
		b.getWormhole().untrackRequest()
		return nil, err
	}

	// Let the provider handle model validation at request time
	// Provider handles all model validation and constraints
	stream := make(chan types.StreamChunk)
	providerFallbacks := append([]TextRoute(nil), b.providerFallbacks...)
	go b.streamWithFallback(ctx, provider, release, b.getProvider(), baseRequest, modelsToTry, providerFallbacks, stream)
	return stream, nil
}

func (b *TextRequestBuilder) streamWithFallback(ctx context.Context, provider types.Provider, release func(), primaryProviderName string, baseRequest *types.TextRequest, modelsToTry []string, providerFallbacks []TextRoute, out chan<- types.StreamChunk) {
	defer close(out)
	defer b.getWormhole().untrackRequest()
	release = sync.OnceFunc(release)
	defer release()

	var failures []string
	var lastErr error
	wormhole := b.getWormhole()
	tryStream := func(provider types.Provider, validationProvider, traceProvider, model string, attempt int, fallback bool) (bool, bool, error) {
		request := cloneTextRequest(baseRequest)
		request.Model = model
		wormhole.emitAttempt(ctx, AttemptEvent{
			Operation: "text.stream",
			Phase:     AttemptStarted,
			Provider:  traceProvider,
			Model:     model,
			Attempt:   attempt,
			Fallback:  fallback,
			Stream:    true,
		})
		if err := wormhole.validateModelAttempt(validationProvider, model, textModelCapabilities, textRequiredCapabilities(request, false, true)); err != nil {
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.stream",
				Phase:     AttemptError,
				Provider:  traceProvider,
				Model:     model,
				Attempt:   attempt,
				Fallback:  fallback,
				Stream:    true,
				Error:     err,
			})
			return false, true, err
		}

		attemptCtx, cancelAttempt := context.WithCancel(ctx)
		stream, err := b.openStream(attemptCtx, cancelAttempt, provider, request)
		if err != nil {
			cancelAttempt()
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.stream",
				Phase:     AttemptError,
				Provider:  traceProvider,
				Model:     model,
				Attempt:   attempt,
				Fallback:  fallback,
				Stream:    true,
				Error:     err,
			})
			return false, true, err
		}

		wormhole.emitStreamEvent(ctx, StreamEvent{
			Type:     StreamStarted,
			Provider: traceProvider,
			Model:    model,
			Attempt:  attempt,
		})

		emitted, retry, err := forwardStreamWithFirstChunkSafety(ctx, cancelAttempt, out, stream)
		cancelAttempt()
		if err != nil {
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.stream",
				Phase:     AttemptError,
				Provider:  traceProvider,
				Model:     model,
				Attempt:   attempt,
				Fallback:  fallback,
				Stream:    true,
				Error:     err,
			})
		} else if emitted {
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.stream",
				Phase:     AttemptSuccess,
				Provider:  traceProvider,
				Model:     model,
				Attempt:   attempt,
				Fallback:  fallback,
				Stream:    true,
			})
			wormhole.emitStreamEvent(ctx, StreamEvent{
				Type:     StreamEnded,
				Provider: traceProvider,
				Model:    model,
				Attempt:  attempt,
			})
		}
		return emitted, retry, err
	}
	emitFinalStreamError := func(provider, model string, attempt int, err error) {
		if err == nil || ctx.Err() != nil {
			return
		}
		wormhole.emitStreamEvent(ctx, StreamEvent{
			Type:     StreamError,
			Provider: provider,
			Model:    model,
			Attempt:  attempt,
			Error:    err,
		})
	}

	attempt := 0
	for _, model := range modelsToTry {
		attempt++
		emitted, retry, err := tryStream(provider, primaryProviderName, provider.Name(), model, attempt, attempt > 1)
		if err != nil {
			lastErr = err
			failures = append(failures, fmt.Sprintf("%s: %v", model, err))
		}
		if emitted || !retry || ctx.Err() != nil {
			emitFinalStreamError(provider.Name(), model, attempt, err)
			return
		}
	}
	release()

	for _, route := range providerFallbacks {
		attempt++
		validationRequest := cloneTextRequest(baseRequest)
		validationRequest.Model = route.Model
		if err := wormhole.validateModelAttempt(route.Provider, route.Model, textModelCapabilities, textRequiredCapabilities(validationRequest, false, true)); err != nil {
			lastErr = err
			failures = append(failures, fmt.Sprintf("%s/%s: %v", route.Provider, route.Model, err))
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.stream",
				Phase:     AttemptStarted,
				Provider:  route.Provider,
				Model:     route.Model,
				Attempt:   attempt,
				Fallback:  true,
				Stream:    true,
			})
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.stream",
				Phase:     AttemptError,
				Provider:  route.Provider,
				Model:     route.Model,
				Attempt:   attempt,
				Fallback:  true,
				Stream:    true,
				Error:     err,
			})
			continue
		}
		fallbackProvider, fallbackRelease, err := wormhole.leaseProvider(route.Provider)
		if err != nil {
			lastErr = err
			failures = append(failures, fmt.Sprintf("%s/%s: %v", route.Provider, route.Model, err))
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.stream",
				Phase:     AttemptStarted,
				Provider:  route.Provider,
				Model:     route.Model,
				Attempt:   attempt,
				Fallback:  true,
				Stream:    true,
			})
			wormhole.emitAttempt(ctx, AttemptEvent{
				Operation: "text.stream",
				Phase:     AttemptError,
				Provider:  route.Provider,
				Model:     route.Model,
				Attempt:   attempt,
				Fallback:  true,
				Stream:    true,
				Error:     err,
			})
			if ctx.Err() != nil {
				return
			}
			continue
		}

		emitted, retry, attemptErr := func() (bool, bool, error) {
			defer fallbackRelease()
			return tryStream(fallbackProvider, route.Provider, route.Provider, route.Model, attempt, true)
		}()
		if attemptErr != nil {
			lastErr = attemptErr
			failures = append(failures, fmt.Sprintf("%s/%s: %v", route.Provider, route.Model, attemptErr))
		}
		if emitted || !retry || ctx.Err() != nil {
			emitFinalStreamError(route.Provider, route.Model, attempt, attemptErr)
			return
		}
	}

	if ctx.Err() != nil {
		return
	}
	if len(modelsToTry)+len(providerFallbacks) == 1 && lastErr != nil {
		sendStreamChunk(ctx, out, types.StreamChunk{Error: lastErr})
		wormhole.emitStreamEvent(ctx, StreamEvent{
			Type:  StreamError,
			Error: lastErr,
		})
		return
	}
	sendStreamChunk(ctx, out, types.StreamChunk{
		Error: fmt.Errorf("all stream attempts failed before emitting a chunk: %s", strings.Join(failures, "; ")),
	})
	wormhole.emitStreamEvent(ctx, StreamEvent{
		Type:  StreamError,
		Error: fmt.Errorf("all stream attempts failed: %s", strings.Join(failures, "; ")),
	})
}
