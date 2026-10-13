package main

import (
	"context"
	"errors"

	wormhole "github.com/garyblankenship/wormhole/v3"
)

func batchExample(ctx context.Context, j *journey) error {
	client := j.client()
	client.EnableAdaptiveConcurrency(&wormhole.EnhancedAdaptiveConfig{AdaptiveConfig: wormhole.AdaptiveConfig{MinCapacity: 1, MaxCapacity: j.cfg.Settings.BatchConcurrency, InitialCapacity: j.cfg.Settings.BatchConcurrency, TargetLatency: j.ttl()}})
	batch := client.Batch().AddAll(j.text(client), j.text(client)).Concurrency(j.cfg.Settings.BatchConcurrency)
	switch j.fixture.mode {
	case "batch-first":
		resp, err := batch.ExecuteFirst(ctx)
		if err = j.expectText(resp, err); err != nil {
			return err
		}
	case "batch-collect":
		responses, errs := batch.ExecuteCollect(ctx)
		if len(errs) != 0 || len(responses) != 2 {
			return errors.New("batch collection lost results")
		}
		for _, resp := range responses {
			if err := j.expectText(resp, nil); err != nil {
				return err
			}
		}
	default:
		results := batch.Execute(ctx)
		if len(results) != 2 {
			return errors.New("batch lost results")
		}
		for i, result := range results {
			if result.Index != i {
				return errors.New("batch ordering mismatch")
			}
			if err := j.expectText(result.Response, result.Error); err != nil {
				return err
			}
		}
	}
	if client.GetAdaptiveLimiter() == nil || client.GetAdaptiveConcurrencyStats() == nil {
		return errors.New("adaptive admission not enabled")
	}
	batch.Clear()
	return require(batch.Count() == 0, "batch clear failed")
}
func idempotencyExample(ctx context.Context, j *journey) error {
	client := j.client(wormhole.WithIdempotencyKey(j.fixture.mode, j.ttl()), wormhole.WithIdempotencyMaxEntries(j.cfg.Settings.CacheCapacity))
	first, err := j.text(client).Generate(ctx)
	if err = j.expectText(first, err); err != nil {
		return err
	}
	first.Text = j.cfg.Prompt
	second, err := j.text(client).Using("openai").Generate(ctx)
	if err = j.expectText(second, err); err != nil {
		return err
	}
	if len(j.fixture.all()) != 1 {
		return errors.New("idempotency replay made another upstream call")
	}
	client.ClearIdempotencyCache()
	third, err := j.text(client).Generate(ctx)
	if err = j.expectText(third, err); err != nil {
		return err
	}
	return require(len(j.fixture.all()) == 2, "idempotency clear did not release retained result")
}
