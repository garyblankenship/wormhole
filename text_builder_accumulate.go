package wormhole

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/v2/types"
)

// StreamAndAccumulate is a convenience method that streams the response while
// accumulating the full text. It returns both the channel for real-time processing
// and a function to get the complete response and any stream-level error after
// streaming finishes.
//
// Example:
//
//	chunks, getResult, err := builder.StreamAndAccumulate(ctx)
//	if err != nil {
//	    return err
//	}
//	for chunk := range chunks {
//	    fmt.Print(chunk.Content())  // Print in real-time
//	}
//	fullText, streamErr := getResult()
//	if streamErr != nil {
//	    // stream ended with an error; fullText is a prefix
//	}
func (b *TextRequestBuilder) StreamAndAccumulate(ctx context.Context) (<-chan types.StreamChunk, func() (string, error), error) {
	stream, err := b.Stream(ctx)
	if err != nil {
		return nil, nil, err
	}

	accumulated := make(chan types.StreamChunk)
	var builder strings.Builder
	var streamErr error
	var mu sync.Mutex

	go func() {
		defer close(accumulated)
		for chunk := range stream {
			mu.Lock()
			if chunk.Error != nil && streamErr == nil {
				streamErr = chunk.Error
			}
			builder.WriteString(chunk.Content())
			mu.Unlock()
			select {
			case accumulated <- chunk:
			case <-ctx.Done():
				// Consumer abandoned the stream; drain the source so the
				// upstream provider goroutine can exit, then stop.
				for range stream {
				}
				return
			}
		}
	}()

	return accumulated, func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		return builder.String(), streamErr
	}, nil
}

// applyStreamIdleTimeout wraps a provider stream with a per-chunk idle watchdog.
// If no chunk arrives within timeout, a typed timeout error is emitted and the
// provider attempt is canceled so its upstream read can close.
func applyStreamIdleTimeout(ctx context.Context, cancel context.CancelFunc, src <-chan types.StreamChunk, timeout time.Duration) <-chan types.StreamChunk {
	out := make(chan types.StreamChunk, cap(src))
	go func() {
		defer close(out)
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		for {
			select {
			case chunk, ok := <-src:
				if !ok {
					return
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				select {
				case <-ctx.Done():
					return
				default:
				}
				select {
				case out <- chunk:
				case <-ctx.Done():
					return
				}
				timer.Reset(timeout)
				if chunk.Error != nil {
					return
				}
			case <-timer.C:
				select {
				case <-ctx.Done():
					return
				default:
				}
				select {
				case out <- types.StreamChunk{
					Error: fmt.Errorf("stream idle timeout: no chunk received within %s", timeout),
				}:
				case <-ctx.Done():
					return
				}
				cancel()
				return
			}
		}
	}()
	return out
}
