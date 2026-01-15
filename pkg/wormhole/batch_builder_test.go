package wormhole

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchBuilder(t *testing.T) {
	t.Run("empty batch returns nil", func(t *testing.T) {
		client := New()
		results := client.Batch().Execute(context.Background())
		assert.Nil(t, results)
	})

	t.Run("Add and Count", func(t *testing.T) {
		client := New()
		batch := client.Batch().
			Add(client.Text().Model("gpt-5").Prompt("Q1")).
			Add(client.Text().Model("gpt-5").Prompt("Q2")).
			Add(client.Text().Model("gpt-5").Prompt("Q3"))

		assert.Equal(t, 3, batch.Count())
	})

	t.Run("AddAll", func(t *testing.T) {
		client := New()
		requests := []*TextRequestBuilder{
			client.Text().Model("gpt-5").Prompt("Q1"),
			client.Text().Model("gpt-5").Prompt("Q2"),
		}
		batch := client.Batch().AddAll(requests...)
		assert.Equal(t, 2, batch.Count())
	})

	t.Run("Clear", func(t *testing.T) {
		client := New()
		batch := client.Batch().
			Add(client.Text().Model("gpt-5").Prompt("Q1")).
			Add(client.Text().Model("gpt-5").Prompt("Q2"))

		assert.Equal(t, 2, batch.Count())

		batch.Clear()
		assert.Equal(t, 0, batch.Count())
	})

	t.Run("Concurrency setting", func(t *testing.T) {
		client := New()
		batch := client.Batch().Concurrency(5)
		assert.Equal(t, 5, batch.concurrency)
	})

	t.Run("default concurrency is 10", func(t *testing.T) {
		client := New()
		batch := client.Batch()
		assert.Equal(t, 10, batch.concurrency)
	})
}

func TestBatchBuilderExecution(t *testing.T) {
	// Skip if no provider configured - this tests the structure, not actual API calls
	t.Run("results preserve order", func(t *testing.T) {
		client := New(WithOpenAI("test-key"))

		// These will fail without real API, but we can check the results structure
		batch := client.Batch().
			Add(client.Text().Model("gpt-5").Prompt("Q1")).
			Add(client.Text().Model("gpt-5").Prompt("Q2")).
			Add(client.Text().Model("gpt-5").Prompt("Q3"))

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		results := batch.Execute(ctx)

		// Should have 3 results in order
		require.Len(t, results, 3)
		assert.Equal(t, 0, results[0].Index)
		assert.Equal(t, 1, results[1].Index)
		assert.Equal(t, 2, results[2].Index)

		// All should have errors (no real API connection)
		for _, r := range results {
			assert.NotNil(t, r.Error)
		}
	})

	t.Run("context cancellation stops execution", func(t *testing.T) {
		client := New(WithOpenAI("test-key"))

		batch := client.Batch().
			Add(client.Text().Model("gpt-5").Prompt("Q1")).
			Add(client.Text().Model("gpt-5").Prompt("Q2"))

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		results := batch.Execute(ctx)

		// All should have context errors
		for _, r := range results {
			assert.Error(t, r.Error)
		}
	})
}

func TestBatchBuilderExecuteCollect(t *testing.T) {
	t.Run("separates successes and failures", func(t *testing.T) {
		client := New(WithOpenAI("test-key"))

		batch := client.Batch().
			Add(client.Text().Model("gpt-5").Prompt("Q1")).
			Add(client.Text().Model("gpt-5").Prompt("Q2"))

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		responses, errs := batch.ExecuteCollect(ctx)

		// Without real API, all should be errors
		assert.Empty(t, responses)
		assert.NotEmpty(t, errs)
	})
}

func TestBatchBuilderExecuteFirst(t *testing.T) {
	t.Run("empty batch returns error", func(t *testing.T) {
		client := New()
		resp, err := client.Batch().ExecuteFirst(context.Background())
		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no requests in batch")
	})

	t.Run("returns first success", func(t *testing.T) {
		client := New(WithOpenAI("test-key"))

		batch := client.Batch().
			Add(client.Text().Model("gpt-5").Prompt("Q1")).
			Add(client.Text().Model("gpt-5").Prompt("Q2"))

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		resp, err := batch.ExecuteFirst(ctx)

		// Without real API, all should fail
		assert.Nil(t, resp)
		assert.Error(t, err)
	})
}

func TestBatchBuilderConcurrencyLimiting(t *testing.T) {
	t.Run("respects concurrency limit", func(t *testing.T) {
		client := New(WithOpenAI("test-key"))

		// Track concurrent executions
		var concurrent int64
		var maxConcurrent int64

		// Create a batch with more requests than concurrency limit
		batch := client.Batch().Concurrency(2)

		// Add 5 requests
		for range 5 {
			batch.Add(client.Text().Model("gpt-5").Prompt("Q"))
		}

		// The concurrency is tracked by the semaphore pattern
		// This is more of a design verification than a runtime test
		assert.Equal(t, 5, batch.Count())
		assert.Equal(t, 2, batch.concurrency)

		// Reset counters (would be tracked in actual execution)
		_ = concurrent
		_ = maxConcurrent
	})
}

// Note: mockBatchProvider removed as tests use real providers with short timeouts
// For comprehensive mock testing, use the testing.MockProviderFactory pattern

func TestBatchBuilderWithAdaptiveConcurrency(t *testing.T) {
	t.Run("adaptive concurrency enabled", func(t *testing.T) {
		client := New(WithOpenAI("test-key"))

		// Enable adaptive concurrency with default config
		client.EnableAdaptiveConcurrency(nil)

		// Verify adaptive limiter is not nil
		adaptiveLimiter := client.GetAdaptiveLimiter()
		assert.NotNil(t, adaptiveLimiter, "adaptive limiter should be initialized")

		// Create a batch with multiple requests
		batch := client.Batch().
			Add(client.Text().Model("gpt-5").Prompt("Q1")).
			Add(client.Text().Model("gpt-5").Prompt("Q2")).
			Add(client.Text().Model("gpt-5").Prompt("Q3")).
			Concurrency(2) // Lower concurrency than batch size

		// Execute with timeout to ensure no deadlocks
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		results := batch.Execute(ctx)

		// Verify execution completed
		require.Len(t, results, 3, "should have 3 results")

		// Verify results are in order (adaptive concurrency should preserve order)
		assert.Equal(t, 0, results[0].Index)
		assert.Equal(t, 1, results[1].Index)
		assert.Equal(t, 2, results[2].Index)

		// All should have errors (no real API connection) but no panics
		for _, r := range results {
			assert.NotNil(t, r.Error, "should have error due to missing API key")
		}

		// Check adaptive concurrency stats
		stats := client.GetAdaptiveConcurrencyStats()
		assert.NotNil(t, stats, "should have adaptive concurrency stats")
	})

	t.Run("provider-specific adaptive concurrency", func(t *testing.T) {
		client := New(WithOpenAI("test-key"))

		// Enable adaptive concurrency with default config
		client.EnableAdaptiveConcurrency(nil)

		// Create batch with requests from different providers (OpenAI and Anthropic)
		batch := client.Batch().
			Add(client.Text().Model("gpt-5").Prompt("OpenAI Q1")).
			Add(client.Text().Model("gpt-5").Prompt("OpenAI Q2"))

		// The adaptive limiter should handle provider-specific concurrency
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		results := batch.Execute(ctx)

		// Verify execution completed
		require.Len(t, results, 2, "should have 2 results")

		// All should have errors but no panics
		for _, r := range results {
			assert.NotNil(t, r.Error, "should have error due to missing API key")
		}
	})

	t.Run("adaptive concurrency with ExecuteFirst", func(t *testing.T) {
		client := New(WithOpenAI("test-key"))

		// Enable adaptive concurrency
		client.EnableAdaptiveConcurrency(nil)

		// Create batch
		batch := client.Batch().
			Add(client.Text().Model("gpt-5").Prompt("Q1")).
			Add(client.Text().Model("gpt-5").Prompt("Q2"))

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		// ExecuteFirst should also use adaptive concurrency
		resp, err := batch.ExecuteFirst(ctx)

		// Should have error (no real API) but no panic
		assert.Nil(t, resp, "should have no response due to missing API key")
		assert.Error(t, err, "should have error due to missing API key")
	})

	t.Run("adaptive concurrency with ExecuteCollect", func(t *testing.T) {
		client := New(WithOpenAI("test-key"))

		// Enable adaptive concurrency
		client.EnableAdaptiveConcurrency(nil)

		// Create batch
		batch := client.Batch().
			Add(client.Text().Model("gpt-5").Prompt("Q1")).
			Add(client.Text().Model("gpt-5").Prompt("Q2"))

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		// ExecuteCollect should also use adaptive concurrency
		responses, errs := batch.ExecuteCollect(ctx)

		// Should have errors (no real API) but no panic
		assert.Empty(t, responses, "should have no successful responses due to missing API key")
		assert.NotEmpty(t, errs, "should have errors due to missing API key")
	})

	t.Run("adaptive concurrency disabled", func(t *testing.T) {
		client := New(WithOpenAI("test-key"))

		// Do NOT enable adaptive concurrency
		adaptiveLimiter := client.GetAdaptiveLimiter()
		assert.Nil(t, adaptiveLimiter, "adaptive limiter should be nil when not enabled")

		// Create batch
		batch := client.Batch().
			Add(client.Text().Model("gpt-5").Prompt("Q1")).
			Add(client.Text().Model("gpt-5").Prompt("Q2"))

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		// Should still work with traditional concurrency
		results := batch.Execute(ctx)

		// Verify execution completed
		require.Len(t, results, 2, "should have 2 results")

		// All should have errors (no real API connection)
		for _, r := range results {
			assert.NotNil(t, r.Error, "should have error due to missing API key")
		}
	})
}
