package wormhole

import (
	"context"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/v3/internal/structured"
	"github.com/garyblankenship/wormhole/v3/types"
)

func (b *TextRequestBuilder) openStream(ctx context.Context, cancel context.CancelFunc, provider types.Provider, request *types.TextRequest, baseURL string) (<-chan types.StreamChunk, error) {
	if err := b.getWormhole().validateFileOwnership(request.Messages, provider.Name()); err != nil {
		return nil, err
	}
	original, _ := b.getWormhole().resolveProviderName(b.getProvider())
	request.Messages = prepareAttemptMessages(request.Messages, original, provider.Name())
	adapted, schema, adaptErr := structured.PrepareText(*request)
	if adaptErr != nil {
		return nil, adaptErr
	}
	request = &adapted
	var stream <-chan types.StreamChunk
	var err error

	ctx = contextWithProviderOperation(ctx, provider, "stream", baseURL)
	if b.getWormhole().providerMiddleware != nil {
		handler := b.getWormhole().providerMiddleware.ApplyStream(provider.Stream)
		stream, err = handler(ctx, *request)
	} else {
		stream, err = provider.Stream(ctx, *request)
	}
	if err != nil {
		return nil, err
	}

	if schema != nil {
		stream = structured.StreamTools(ctx, schema, stream)
	}
	// Apply per-chunk idle timeout if configured.
	if timeout := b.getWormhole().config.StreamIdleTimeout; timeout > 0 {
		stream = applyStreamIdleTimeout(ctx, cancel, stream, timeout)
	}
	return stream, nil
}

// streamAttemptResult records delivery at the public channel boundary. Even a
// metadata-only chunk commits the attempt because the consumer can observe it.
// Failed attempts may fall back only while nothing has been delivered.
type streamAttemptResult struct {
	delivered bool
	err       error
}

func (r streamAttemptResult) canFallback(ctx context.Context) bool {
	return !r.delivered && r.err != nil && ctx.Err() == nil
}

func forwardStreamWithFirstChunkSafety(ctx context.Context, cancelAttempt context.CancelFunc, out chan<- types.StreamChunk, stream <-chan types.StreamChunk) streamAttemptResult {
	var result streamAttemptResult
	for {
		select {
		case <-ctx.Done():
			result.err = ctx.Err()
			return result
		case chunk, ok := <-stream:
			if !ok {
				if !result.delivered {
					result.err = fmt.Errorf("stream closed before first chunk")
				}
				return result
			}
			if !result.delivered && chunk.HasError() {
				cancelAttempt()
				go drainStream(ctx, stream, time.Second)
				result.err = chunk.Error
				return result
			}
			if !sendStreamChunk(ctx, out, chunk) {
				result.err = ctx.Err()
				return result
			}
			result.delivered = true
			if chunk.HasError() {
				result.err = chunk.Error
				return result
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

func drainStream(ctx context.Context, stream <-chan types.StreamChunk, maxDuration time.Duration) {
	timer := time.NewTimer(maxDuration)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
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
	request.SystemPrompt = ""
}
