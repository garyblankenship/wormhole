package wormhole

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v2/types"
)

func (b *TextRequestBuilder) openStream(ctx context.Context, cancel context.CancelFunc, provider types.Provider, request *types.TextRequest) (<-chan types.StreamChunk, error) {
	var stream <-chan types.StreamChunk
	var err error

	ctx = contextWithProviderOperation(ctx, provider, "stream")
	if b.getWormhole().providerMiddleware != nil {
		handler := b.getWormhole().providerMiddleware.ApplyStream(provider.Stream)
		stream, err = handler(ctx, *request)
	} else {
		stream, err = provider.Stream(ctx, *request)
	}
	if err != nil {
		return nil, err
	}

	// Apply per-chunk idle timeout if configured.
	if timeout := b.getWormhole().config.StreamIdleTimeout; timeout > 0 {
		stream = applyStreamIdleTimeout(ctx, cancel, stream, timeout)
	}
	return stream, nil
}

func forwardStreamWithFirstChunkSafety(ctx context.Context, cancelAttempt context.CancelFunc, out chan<- types.StreamChunk, stream <-chan types.StreamChunk) (emitted bool, retry bool, err error) {
	for {
		select {
		case <-ctx.Done():
			return false, false, ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				if !emitted {
					return false, true, fmt.Errorf("stream closed before first chunk")
				}
				return true, false, nil
			}
			if !emitted && chunk.HasError() {
				cancelAttempt()
				go drainStream(ctx, stream)
				return false, true, chunk.Error
			}
			emitted = true
			if !sendStreamChunk(ctx, out, chunk) {
				return true, false, ctx.Err()
			}
			if chunk.HasError() {
				return true, false, chunk.Error
			}
		}
	}
}

func sendStreamChunk(ctx context.Context, out chan<- types.StreamChunk, chunk types.StreamChunk) bool {
	select {
	case out <- chunk:
		return true
	case <-ctx.Done():
		return false
	}
}

func drainStream(ctx context.Context, stream <-chan types.StreamChunk) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-stream:
			if !ok {
				return
			}
		}
	}
}

func cloneTextRequest(src *types.TextRequest) *types.TextRequest {
	if src == nil {
		return &types.TextRequest{}
	}

	cloned := &types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: src.Model,
		},
		SystemPrompt:   src.SystemPrompt,
		ResponseFormat: types.CloneValue(src.ResponseFormat),
	}

	cloneBaseRequestFields(&cloned.BaseRequest, &src.BaseRequest)
	if src.ToolChoice != nil {
		toolChoice := *src.ToolChoice
		cloned.ToolChoice = &toolChoice
	}
	cloned.Messages = types.CloneMessages(src.Messages)
	cloned.Tools = types.CloneTools(src.Tools)

	return cloned
}

func prepareTextExecutionRequest(request *types.TextRequest) {
	if request == nil {
		return
	}
	request.Messages = prepareExecutionMessages(request.SystemPrompt, request.Messages)
}
