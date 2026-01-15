package wormhole

import (
	"context"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
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

	// Default concurrency
	concurrency := b.concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	// Limit concurrency to number of requests
	if concurrency > len(b.requests) {
		concurrency = len(b.requests)
	}

	results := make([]BatchResult, len(b.requests))
	taskCh := make(chan int, len(b.requests)) // send indices to workers
	resultCh := make(chan batchResult, len(b.requests))
	var wg sync.WaitGroup

	// Check if adaptive concurrency is enabled
	adaptiveLimiter := b.wormhole.GetAdaptiveLimiter()

	// Start worker goroutines
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range taskCh {
				req := b.requests[index]

				var resp *types.TextResponse
				var err error
				var start time.Time

				// Use adaptive concurrency if enabled
				if adaptiveLimiter != nil {
					// Extract provider and model from request
					provider := req.provider
					model := req.request.Model

					// Acquire slot with provider/model awareness
					if !adaptiveLimiter.AcquireWithProvider(ctx, provider, model) {
						// Context expired or cancelled
						resultCh <- batchResult{
							index:    index,
							response: nil,
							err:      ctx.Err(),
						}
						continue
					}

					// Record start time
					start = time.Now()
					resp, err = req.Generate(ctx)

					// Record latency and error
					latency := time.Since(start)
					adaptiveLimiter.RecordLatencyWithProvider(latency, provider, model, err)

					// Release slot
					adaptiveLimiter.ReleaseWithProvider(provider, model)
				} else {
					// Use traditional fixed concurrency
					resp, err = req.Generate(ctx)
				}

				resultCh <- batchResult{
					index:    index,
					response: resp,
					err:      err,
				}
			}
		}()
	}

	// Send all tasks
	for i := range b.requests {
		taskCh <- i
	}
	close(taskCh)

	// Wait for workers to finish and collect results
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results (order doesn't matter, we store by index)
	for r := range resultCh {
		results[r.index] = BatchResult{
			Index:    r.index,
			Response: r.response,
			Error:    r.err,
		}
	}

	return results
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

	// Determine concurrency
	concurrency := b.concurrency
	if concurrency <= 0 {
		concurrency = 10
	}
	if concurrency > len(b.requests) {
		concurrency = len(b.requests)
	}

	// Check if adaptive concurrency is enabled
	adaptiveLimiter := b.wormhole.GetAdaptiveLimiter()

	type result struct {
		resp *types.TextResponse
		err  error
	}

	resultCh := make(chan result, len(b.requests))
	taskCh := make(chan int, len(b.requests))
	var wg sync.WaitGroup

	// Start workers
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range taskCh {
				req := b.requests[index]

				var resp *types.TextResponse
				var err error
				var start time.Time

				// Use adaptive concurrency if enabled
				if adaptiveLimiter != nil {
					// Extract provider and model from request
					provider := req.provider
					model := req.request.Model

					// Acquire slot with provider/model awareness
					if !adaptiveLimiter.AcquireWithProvider(ctx, provider, model) {
						// Context expired or cancelled, send error and continue
						select {
						case resultCh <- result{nil, ctx.Err()}:
						case <-ctx.Done():
						}
						continue
					}

					// Record start time
					start = time.Now()
					resp, err = req.Generate(ctx)

					// Record latency and error
					latency := time.Since(start)
					adaptiveLimiter.RecordLatencyWithProvider(latency, provider, model, err)

					// Release slot
					adaptiveLimiter.ReleaseWithProvider(provider, model)
				} else {
					// Use traditional fixed concurrency
					resp, err = req.Generate(ctx)
				}

				select {
				case resultCh <- result{resp, err}:
				case <-ctx.Done():
				}
			}
		}()
	}

	// Send all tasks
	for i := range b.requests {
		taskCh <- i
	}
	close(taskCh)

	// Wait for workers to finish (in background)
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Wait for first success or all failures
	var lastErr error
	for i := 0; i < len(b.requests); i++ {
		select {
		case r := <-resultCh:
			if r.err == nil {
				cancel() // Cancel remaining requests
				return r.resp, nil
			}
			lastErr = r.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, lastErr
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
