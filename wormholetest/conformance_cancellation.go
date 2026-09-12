package wormholetest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func checkStreamCancellationAfterStart(provider types.Provider, model string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	stream, err := provider.Stream(ctx, types.TextRequest{
		BaseRequest: types.BaseRequest{Model: model},
		Messages:    []types.Message{types.NewUserMessage("hello")},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("Stream did not start before timeout: %w", err)
		}
		return fmt.Errorf("Stream returned error before cancellation: %w", err)
	}
	if stream == nil {
		return fmt.Errorf("Stream returned nil channel")
	}

	if err := waitForFirstStreamChunk(stream, timeout); err != nil {
		return err
	}
	cancel()
	if err := waitForStreamClosure(stream, timeout); err != nil {
		return err
	}
	return nil
}

func waitForFirstStreamChunk(stream <-chan types.TextChunk, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case chunk, ok := <-stream:
		if !ok {
			return fmt.Errorf("Stream closed before its first chunk")
		}
		if chunk.Error != nil {
			return fmt.Errorf("Stream returned error before cancellation: %w", chunk.Error)
		}
		return nil
	case <-timer.C:
		return fmt.Errorf("Stream did not produce its first chunk before timeout")
	}
}

func waitForStreamClosure(stream <-chan types.TextChunk, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case _, ok := <-stream:
			if !ok {
				return nil
			}
		case <-timer.C:
			return fmt.Errorf("Stream did not close promptly after cancellation")
		}
	}
}
