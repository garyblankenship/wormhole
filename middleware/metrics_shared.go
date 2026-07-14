package middleware

import (
	"context"
	"time"
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
