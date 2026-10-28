package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func withMeasuredRequest[Req any, Resp any](
	ctx context.Context,
	request Req,
	handler func(context.Context, Req) (Resp, error),
	record func(Resp, error, time.Duration),
) (Resp, error) {
	start := time.Now()
	resp, err := handler(ctx, request)
	if record != nil {
		record(resp, err, time.Since(start))
	}
	return resp, err
}

// withMeasuredStream settles only when setup fails or the forwarding goroutine
// exits on upstream closure or caller cancellation.
func withMeasuredStream(
	ctx context.Context,
	request types.TextRequest,
	handler types.StreamHandler,
	estimateTokens bool,
	record func(error, time.Duration, int, int),
) (<-chan types.TextChunk, error) {
	start := time.Now()
	upstream, err := handler(ctx, request)
	if err != nil {
		inputTokens := 0
		if estimateTokens {
			inputTokens = estimateInputTokens(request.Messages)
		}
		record(err, time.Since(start), inputTokens, 0)
		return upstream, err
	}

	out := make(chan types.TextChunk)
	go func() {
		// Upstream closure or ctx cancellation terminates this forwarder.
		defer close(out)
		var firstErr error
		var text strings.Builder
		var inputTokens, outputTokens int
		var reportedUsage bool
		defer func() {
			if !reportedUsage && estimateTokens {
				inputTokens = estimateInputTokens(request.Messages)
				outputTokens = estimateTextTokens(text.String())
			}
			record(firstErr, time.Since(start), inputTokens, outputTokens)
		}()
		for {
			select {
			case <-ctx.Done():
				if firstErr == nil {
					firstErr = ctx.Err()
				}
				return
			case chunk, ok := <-upstream:
				if !ok {
					return
				}
				if firstErr == nil && chunk.Error != nil {
					firstErr = chunk.Error
				}
				if chunk.Usage != nil {
					reportedUsage = true
					inputTokens = max(inputTokens, chunk.Usage.PromptTokens)
					outputTokens = max(outputTokens, chunk.Usage.CompletionTokens)
				}
				if estimateTokens {
					text.WriteString(chunk.Content())
				}
				select {
				case out <- chunk:
				case <-ctx.Done():
					if firstErr == nil {
						firstErr = ctx.Err()
					}
					return
				}
			}
		}
	}()
	return out, nil
}

func requestLabelsFromContext(ctx context.Context, method, model string) *RequestLabels {
	provider := "unknown"

	if ctx != nil {
		if p, ok := ctx.Value(CtxKeyWormholeProvider).(string); ok && p != "" {
			provider = p
		} else if p, ok := ctx.Value(CtxKeyProvider).(string); ok && p != "" {
			provider = p
		}

		if method == "" {
			if m, ok := ctx.Value(CtxKeyMethod).(string); ok {
				method = m
			}
		}
		if model == "" {
			if m, ok := ctx.Value(CtxKeyModel).(string); ok {
				model = m
			}
		}
	}

	if method == "" && model == "" && provider == "unknown" {
		return nil
	}

	return &RequestLabels{
		Provider:  provider,
		Model:     model,
		Method:    method,
		ErrorType: "",
	}
}
