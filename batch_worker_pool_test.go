package wormhole

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestBatchExecutionModesShareWorkerPool(t *testing.T) {
	t.Parallel()
	run := func(name string, fn func(*testing.T)) {
		t.Run(name, func(t *testing.T) { t.Parallel(); synctest.Test(t, fn) })
	}
	run("worker pool enforces configured concurrency", func(t *testing.T) {
		var active, maximum atomic.Int32
		client := newBatchTestClient(func(_ context.Context, request types.TextRequest) (*types.TextResponse, error) {
			current := active.Add(1)
			defer active.Add(-1)
			for observed := maximum.Load(); current > observed && !maximum.CompareAndSwap(observed, current); observed = maximum.Load() {
			}
			<-time.After(20 * time.Millisecond)
			return &types.TextResponse{Model: request.Model}, nil
		})
		t.Cleanup(func() { _ = client.Close() })
		batch := client.Batch().Concurrency(2)
		for range 5 {
			batch.Add(client.Text().Model("model").Prompt("request"))
		}
		results := batch.Execute(t.Context())
		require.Len(t, results, 5)
		assert.Equal(t, int32(2), maximum.Load())
	})
	run("Execute preserves request order when completion order differs", func(t *testing.T) {
		client := newBatchTestClient(func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
			if request.Model == "slow" {
				<-time.After(20 * time.Millisecond)
			}
			return &types.TextResponse{Model: request.Model}, nil
		})
		t.Cleanup(func() { _ = client.Close() })
		results := client.Batch().Concurrency(2).Add(client.Text().Model("slow").Prompt("first")).Add(client.Text().Model("fast").Prompt("second")).Execute(t.Context())
		require.Len(t, results, 2)
		require.NoError(t, results[0].Error)
		require.NoError(t, results[1].Error)
		assert.Equal(t, "slow", results[0].Response.Model)
		assert.Equal(t, "fast", results[1].Response.Model)
	})
	run("ExecuteFirst cancels remaining requests after a success", func(t *testing.T) {
		started, canceled := make(chan struct{}), make(chan struct{})
		client := newBatchTestClient(func(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
			if request.Model == "fast" {
				select {
				case <-started:
					return &types.TextResponse{Model: request.Model}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			close(started)
			<-ctx.Done()
			close(canceled)
			return nil, ctx.Err()
		})
		t.Cleanup(func() { _ = client.Close() })
		response, err := client.Batch().Concurrency(2).Add(client.Text().Model("slow").Prompt("first")).Add(client.Text().Model("fast").Prompt("second")).ExecuteFirst(t.Context())
		require.NoError(t, err)
		require.NotNil(t, response)
		assert.Equal(t, "fast", response.Model)
		synctest.Wait()
		select {
		case <-canceled:
		default:
			t.Fatal("started request was not canceled")
		}
	})
	run("ExecuteFirst returns the last serial failure", func(t *testing.T) {
		client := newBatchTestClient(func(_ context.Context, request types.TextRequest) (*types.TextResponse, error) {
			return nil, errors.New(request.Model)
		})
		t.Cleanup(func() { _ = client.Close() })
		response, err := client.Batch().Concurrency(1).Add(client.Text().Model("first-error").Prompt("first")).Add(client.Text().Model("last-error").Prompt("second")).ExecuteFirst(t.Context())
		assert.Nil(t, response)
		require.EqualError(t, err, "last-error")
	})
}
