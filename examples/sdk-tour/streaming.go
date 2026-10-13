package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
)

func consume(ctx context.Context, b *wormhole.TextRequestBuilder) (string, error) {
	stream, err := b.Stream(ctx)
	if err != nil {
		return "", err
	}
	var content strings.Builder
	done := false
	for chunk := range stream {
		if chunk.Error != nil {
			return "", chunk.Error
		}
		content.WriteString(chunk.Content())
		done = done || chunk.IsDone()
	}
	if !done {
		return content.String(), errors.New("stream omitted terminal completion")
	}
	return content.String(), nil
}
func streamExample(ctx context.Context, j *journey) error {
	var mu sync.Mutex
	var events []wormhole.StreamEvent
	client := j.client(wormhole.WithStreamTrace(func(_ context.Context, e wormhole.StreamEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	}))
	got, err := consume(ctx, j.text(client))
	if err != nil {
		return err
	}
	if got != j.cfg.Answer {
		return errors.New("stream content mismatch")
	}
	mu.Lock()
	defer mu.Unlock()
	terminal := 0
	started := 0
	for _, e := range events {
		if e.Type == wormhole.StreamStarted {
			started++
		}
		if e.Type == wormhole.StreamEnded {
			terminal++
		}
		if e.Type == wormhole.StreamError {
			return e.Error
		}
	}
	return require(started == 1 && terminal == 1, "stream lifecycle mismatch")
}
func accumulateExample(ctx context.Context, j *journey) error {
	client := j.client()
	stream, full, err := j.text(client).StreamAndAccumulate(ctx)
	if err != nil {
		return err
	}
	var seen strings.Builder
	for chunk := range stream {
		if chunk.Error != nil {
			return chunk.Error
		}
		seen.WriteString(chunk.Content())
	}
	got, err := full()
	if err != nil {
		return err
	}
	return require(got == j.cfg.Answer && seen.String() == got, "accumulated stream differs from observed content")
}
func streamErrorExample(ctx context.Context, j *journey) error {
	client := j.client()
	_, err := consume(ctx, j.text(client))
	return require(err != nil, "truncated stream silently succeeded")
}
func incompleteExample(ctx context.Context, j *journey) error {
	client := j.client()
	_, err := j.text(client).Generate(ctx)
	var incomplete *types.IncompleteGenerationError
	return require(errors.As(err, &incomplete), fmt.Sprintf("expected incomplete-generation error, got %v", err))
}
