package wormhole

import (
	"context"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// BatchBuilder executes multiple text generation requests concurrently.
//
// Example:
//
//	results := client.Batch().
//	    Add(client.Text().Model("gpt-4o").Prompt("Q1")).
//	    Add(client.Text().Model("gpt-4o").Prompt("Q2")).
//	    Concurrency(5).
//	    Execute(ctx)
//
//	for _, result := range results {
//	    if result.Error != nil {
//	        log.Printf("Request %d failed: %v", result.Index, result.Error)
//	    } else {
//	        fmt.Printf("Response %d: %s\n", result.Index, result.Response.Content())
//	    }
//	}
type BatchBuilder struct {
	wormhole    *Wormhole
	requests    []*TextRequestBuilder
	concurrency int
}

// BatchResult holds the result of a single request in a batch.
type BatchResult struct {
	Index    int                 // Original index of the request
	Response *types.TextResponse // Response if successful
	Error    error               // Error if failed
}

// Add adds a text request builder to the batch.
// The builder should be fully configured but not executed.
func (b *BatchBuilder) Add(request *TextRequestBuilder) *BatchBuilder {
	b.requests = append(b.requests, request)
	return b
}

// AddAll adds multiple text request builders to the batch.
func (b *BatchBuilder) AddAll(requests ...*TextRequestBuilder) *BatchBuilder {
	b.requests = append(b.requests, requests...)
	return b
}

// Concurrency sets the maximum number of concurrent requests.
// Default is 10. Set to 0 for unlimited (not recommended).
func (b *BatchBuilder) Concurrency(n int) *BatchBuilder {
	b.concurrency = n
	return b
}

// Execute runs all requests concurrently and returns results.
// Results are returned in the same order as requests were added.
// All requests complete before Execute returns - it waits for all.
func (b *BatchBuilder) Execute(ctx context.Context) []BatchResult {
	if len(b.requests) == 0 {
		return nil
	}

	results := make([]BatchResult, len(b.requests))
	// Collect results (order doesn't matter, we store by index)
	for r := range b.execute(ctx) {
		results[r.index] = BatchResult{
			Index:    r.index,
			Response: r.response,
			Error:    r.err,
		}
	}

	return results
}

func executeRequestWithLimiter(ctx context.Context, req *TextRequestBuilder, limiter *EnhancedAdaptiveLimiter) (*types.TextResponse, error) {
	if limiter == nil {
		return req.Generate(ctx)
	}

	provider := req.provider
	model := req.request.Model

	release, ok := limiter.AcquireTokenWithProvider(ctx, provider, model)
	if !ok {
		return nil, ctx.Err()
	}
	defer release()

	start := time.Now()
	resp, err := req.Generate(ctx)

	latency := time.Since(start)
	limiter.RecordLatencyWithProvider(latency, provider, model, err)

	return resp, err
}

// batchResult internal struct for worker results
type batchResult struct {
	index    int
	response *types.TextResponse
	err      error
}

// ExecuteCollect runs all requests and returns only successful responses.
// Errors are collected separately. Useful when you want to process
// successful results and handle errors separately.
func (b *BatchBuilder) ExecuteCollect(ctx context.Context) (responses []*types.TextResponse, errors []error) {
	results := b.Execute(ctx)

	for _, r := range results {
		if r.Error != nil {
			errors = append(errors, r.Error)
		} else {
			responses = append(responses, r.Response)
		}
	}

	return responses, errors
}

// ExecuteFirst runs all requests and returns the first successful response.
// Useful for racing multiple models or redundant requests.
func (b *BatchBuilder) ExecuteFirst(ctx context.Context) (*types.TextResponse, error) {
	if len(b.requests) == 0 {
		return nil, types.ErrInvalidRequest.WithDetails("no requests in batch")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Wait for first success or all failures
	var lastErr error
	results := b.execute(ctx)
	for {
		select {
		case r, ok := <-results:
			if !ok {
				return nil, lastErr
			}
			if r.err == nil {
				cancel() // Cancel remaining requests
				return r.response, nil
			}
			lastErr = r.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// execute owns the bounded worker pool shared by the batch completion modes.
// Its buffered result stream lets a caller stop consuming after cancellation
// without stranding workers that are finishing already-started requests.
func (b *BatchBuilder) execute(ctx context.Context) <-chan batchResult {
	resultCh := make(chan batchResult, len(b.requests))
	if len(b.requests) == 0 {
		close(resultCh)
		return resultCh
	}

	concurrency := b.concurrency
	if concurrency <= 0 {
		concurrency = 10
	}
	if concurrency > len(b.requests) {
		concurrency = len(b.requests)
	}

	taskCh := make(chan int, len(b.requests))
	for i := range b.requests {
		taskCh <- i
	}
	close(taskCh)

	adaptiveLimiter := b.wormhole.GetAdaptiveLimiter()
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for range concurrency {
		go func() {
			defer wg.Done()
			for index := range taskCh {
				response, err := executeRequestWithLimiter(ctx, b.requests[index], adaptiveLimiter)
				if err == ctx.Err() && response == nil {
					err = ctx.Err()
				}
				resultCh <- batchResult{index: index, response: response, err: err}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()
	return resultCh
}

// Count returns the number of requests in the batch.
func (b *BatchBuilder) Count() int {
	return len(b.requests)
}

// Clear removes all requests from the batch.
func (b *BatchBuilder) Clear() *BatchBuilder {
	b.requests = nil
	return b
}
