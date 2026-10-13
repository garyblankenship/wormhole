package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	wormhole "github.com/garyblankenship/wormhole/v3"
)

func fallbackExample(ctx context.Context, j *journey) error {
	var mu sync.Mutex
	var events []wormhole.AttemptEvent
	client := j.client(wormhole.WithAttemptTrace(func(_ context.Context, e wormhole.AttemptEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	}))
	builder := client.Text().Model(j.cfg.Models["fail"]).Prompt(j.cfg.Prompt)
	if j.fixture.mode == "provider-fallback" {
		builder.WithProviderFallback(wormhole.TextRoute{Provider: "backup", Model: j.cfg.Models["fallback"]})
	} else {
		builder.WithFallback(j.cfg.Models["fallback"])
	}
	resp, err := builder.Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	if resp.Model != j.cfg.Models["fallback"] {
		return errors.New("fallback selected wrong model")
	}
	requests := j.fixture.all()
	if len(requests) != 2 || str(requests[0].Body["model"]) != j.cfg.Models["fail"] || str(requests[1].Body["model"]) != j.cfg.Models["fallback"] {
		return errors.New("fallback request route mismatch")
	}
	mu.Lock()
	defer mu.Unlock()
	return require(len(events) == 4 && events[1].Phase == wormhole.AttemptError && events[3].Phase == wormhole.AttemptSuccess && events[3].Fallback, "attempt trace mismatch")
}
func retryExample(ctx context.Context, j *journey) error {
	config := j.config("/v1").WithRetries(j.cfg.Settings.Retries, time.Duration(j.cfg.Settings.RetryDelayMS)*time.Millisecond)
	if j.fixture.mode == "key-rotation" {
		config.NoAuth = false
		config.APIKeys = []string{j.cfg.FakeKeys[0], j.cfg.FakeKeys[1]}
	}
	client := j.client(wormhole.WithOpenAI("", config))
	resp, err := j.text(client).Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	requests := j.fixture.all()
	if len(requests) != 2 {
		return fmt.Errorf("expected one retry, got %d requests", len(requests))
	}
	if j.fixture.mode == "key-rotation" {
		return require(requests[0].Authorization == "Bearer "+j.cfg.FakeKeys[0] && requests[1].Authorization == "Bearer "+j.cfg.FakeKeys[1], "429 did not rotate fixture key")
	}
	return nil
}
