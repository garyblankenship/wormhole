package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/middleware"
	"github.com/garyblankenship/wormhole/v3/types"
)

func cancellationExample(ctx context.Context, j *journey) error {
	client := j.client()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := j.text(client).Generate(canceled)
	if !errors.Is(err, context.Canceled) {
		return fmt.Errorf("cancellation identity lost: %v", err)
	}
	expired, stop := context.WithDeadline(ctx, time.Unix(0, 0))
	defer stop()
	_, err = j.text(client).Generate(expired)
	if !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("deadline identity lost: %v", err)
	}
	mw := middleware.NewTypedRateLimitMiddleware(j.cfg.Settings.RatePerSecond)
	called := false
	_, err = mw.ApplyText(func(context.Context, types.TextRequest) (*types.TextResponse, error) { called = true; return nil, nil })(canceled, types.TextRequest{})
	return require(err != nil && !called && len(j.fixture.all()) == 0, "canceled request reached provider/middleware")
}
func idleExample(ctx context.Context, j *journey) error {
	client := j.client(wormhole.WithStreamIdleTimeout(time.Duration(j.cfg.Settings.IdleMS) * time.Millisecond))
	_, err := consume(ctx, j.text(client))
	return require(err != nil, "stalled stream ignored configured idle deadline")
}
func handlesExample(ctx context.Context, j *journey) error {
	client := j.client()
	handle, err := client.ProviderWithHandle("openai")
	if err != nil {
		return err
	}
	resp, err := handle.Text(ctx, types.TextRequest{BaseRequest: types.BaseRequest{Model: j.cfg.Models["text"]}, Messages: []types.Message{types.NewUserMessage(j.cfg.Prompt)}})
	if err = j.expectText(resp, err); err != nil {
		_ = handle.Close()
		return err
	}
	if err = handle.Close(); err != nil {
		return err
	}
	client.CleanupStaleProviders(0, 0)
	if client.GetCacheMetrics().Evictions == 0 {
		return errors.New("provider handle did not release eviction lease")
	}
	if err = client.Shutdown(ctx); err != nil {
		return err
	}
	_, err = j.text(client).Generate(ctx)
	return require(client.IsShuttingDown() && err != nil, "shutdown allowed new requests")
}
func healthExample(ctx context.Context, j *journey) error {
	client := j.client()
	checker := middleware.NewHealthChecker(j.ttl())
	checked := make(chan error, 1)
	checker.SetCheckFunction(func(checkCtx context.Context, provider string) error {
		resp, err := j.text(client).Using(provider).Generate(checkCtx)
		err = j.expectText(resp, err)
		checked <- err
		return err
	})
	checker.Start([]string{"openai"})
	defer checker.Stop()
	select {
	case err := <-checked:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return require(checker.IsHealthy("openai") && len(checker.GetHealthyProviders([]string{"openai"})) == 1, "healthy provider excluded")
}
