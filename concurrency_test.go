package wormhole

import (
	"errors"
	"sync"
	"testing"

	"github.com/garyblankenship/wormhole/v3/types"
)

// TestConcurrentProviderAccess tests that multiple goroutines can safely
// access ProviderWithHandle simultaneously without causing data races
func TestConcurrentProviderAccess(t *testing.T) {
	t.Parallel()
	// Create wormhole with OpenAI provider configured
	w := New(
		WithDefaultProvider("openai"),
		WithOpenAI("test-key"),
	)

	const numGoroutines = 100
	const numIterations = 10

	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines*numIterations)

	// Launch multiple goroutines that simultaneously acquire provider handles.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				handle, err := w.ProviderWithHandle("openai")
				if err != nil {
					errChan <- err
					return
				}
				if handle == nil {
					errChan <- errors.New("nil provider handle")
					return
				}
				if err := handle.Close(); err != nil {
					errChan <- err
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			t.Fatalf("Concurrent provider access failed: %v", err)
		}
	}
}

// TestConcurrentProviderBuilders tests that multiple goroutines can safely
// use builder methods simultaneously without causing data races
func TestConcurrentProviderBuilders(t *testing.T) {
	t.Parallel()
	// Create wormhole with multiple providers
	w := New(
		WithDefaultProvider("openai"),
		WithOpenAI("test-key-openai"),
		WithAnthropic("test-key-anthropic"),
	)

	const numGoroutines = 50
	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines)

	// Launch multiple goroutines that build requests with different providers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()

			// Alternate between providers to create more contention
			providerName := "openai"
			if routineID%2 == 0 {
				providerName = "anthropic"
			}

			// Create a text request builder
			builder := w.Text().Using(providerName).Model("test-model").Prompt("test prompt")
			if builder == nil {
				errChan <- errors.New("text builder is nil")
				return
			}

			// Create embeddings builder
			embBuilder := w.Embeddings().Using(providerName).Model("test-model").Input("test input")
			if embBuilder == nil {
				errChan <- errors.New("embeddings builder is nil")
				return
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			t.Fatalf("Concurrent builder access failed: %v", err)
		}
	}
}

// TestConcurrentOptionCreation tests that multiple clients can be created concurrently
// using functional options pattern (testing that our new immutable design is thread-safe)
func TestConcurrentOptionCreation(t *testing.T) {
	t.Parallel()
	const numGoroutines = 50
	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines)

	// Launch multiple goroutines that create Wormhole instances concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()

			// Each goroutine creates a uniquely configured client
			var w *Wormhole
			switch routineID % 2 {
			case 0:
				w = New(
					WithDefaultProvider("gemini"),
					WithGemini("test-key"),
				)
			case 1:
				w = New(
					WithDefaultProvider("ollama"),
					WithOllama(types.ProviderConfig{
						APIKey:  "test-key",
						BaseURL: "http://localhost:11434",
					}),
				)
			}

			if w == nil {
				errChan <- errors.New("failed to create Wormhole instance")
				return
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			t.Fatalf("Concurrent option-based client creation failed: %v", err)
		}
	}
}

// TestRaceConditionScenario simulates the exact scenario from the bug report:
// Multiple goroutines making concurrent requests to the same provider
func TestRaceConditionScenario(t *testing.T) {
	t.Parallel()
	w := New(
		WithDefaultProvider("openai"),
		WithOpenAI("test-key"),
	)

	const numGoroutines = 100
	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines)

	// Simulate the exact scenario: multiple goroutines making text generation requests
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()

			// This simulates the exact call pattern that caused the bug
			builder := w.Text().Model("gpt-4").Prompt("You are a helpful assistant")
			if builder == nil {
				errChan <- errors.New("text builder is nil")
				return
			}

			// Acquire through the same leased provider path used by requests.
			provider, release, err := builder.getProviderWithBaseURL()
			if err != nil {
				errChan <- err
				return
			}
			if release == nil {
				errChan <- errors.New("provider release is nil")
				return
			}
			defer release()
			if provider == nil {
				errChan <- errors.New("provider is nil")
				return
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			t.Fatalf("Race condition scenario failed: %v", err)
		}
	}
}

// TestHighContentionProviderAccess creates maximum contention by having
// all goroutines access the same provider at exactly the same time
func TestHighContentionProviderAccess(t *testing.T) {
	t.Parallel()
	w := New(
		WithDefaultProvider("openai"),
		WithOpenAI("test-key"),
	)

	const numGoroutines = 200
	var wg sync.WaitGroup
	var startWg sync.WaitGroup
	errChan := make(chan error, numGoroutines)

	startWg.Add(1) // Used to synchronize start time

	// Launch all goroutines but make them wait
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Wait for signal to start (creates maximum contention)
			startWg.Wait()

			// All goroutines hit this at the same time
			handle, err := w.ProviderWithHandle("openai")
			if err != nil {
				errChan <- err
				return
			}
			if handle == nil {
				errChan <- errors.New("nil provider handle")
				return
			}
			if err := handle.Close(); err != nil {
				errChan <- err
				return
			}
		}()
	}

	// Release all goroutines at once
	startWg.Done()

	// Wait for all to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			t.Fatalf("High contention provider access failed: %v", err)
		}
	}
}

// TestConcurrentProviderInitialization tests the double-checked locking pattern
func TestConcurrentProviderInitialization(t *testing.T) {
	t.Parallel()
	const numTests = 10

	// Run the test multiple times to increase chance of catching race conditions
	for testRun := 0; testRun < numTests; testRun++ {
		// Create a fresh wormhole for each test to ensure clean state
		w := New(
			WithDefaultProvider("openai"),
			WithOpenAI("test-key"),
			WithAnthropic("test-key-anthropic"),
		)

		const numGoroutines = 50
		var wg sync.WaitGroup
		var startWg sync.WaitGroup
		errChan := make(chan error, numGoroutines)
		providerChan := make(chan types.Provider, numGoroutines)

		startWg.Add(1)

		// All goroutines try to initialize the same provider simultaneously
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				startWg.Wait() // Synchronize start

				handle, err := w.ProviderWithHandle("openai")
				if err != nil {
					errChan <- err
					return
				}
				providerChan <- handle.Provider
				if err := handle.Close(); err != nil {
					errChan <- err
				}
			}()
		}

		startWg.Done() // Release all goroutines
		wg.Wait()
		close(errChan)
		close(providerChan)

		// Check for errors
		for err := range errChan {
			if err != nil {
				t.Fatalf("Test run %d failed: %v", testRun, err)
			}
		}

		// Verify all goroutines got the same provider instance (should be cached)
		var firstProvider types.Provider
		providerCount := 0
		for provider := range providerChan {
			if firstProvider == nil {
				firstProvider = provider
			} else if provider != firstProvider {
				t.Fatalf("Test run %d: Different provider instances returned, caching failed", testRun)
			}
			providerCount++
		}

		if providerCount != numGoroutines {
			t.Fatalf("Test run %d: Expected %d providers, got %d", testRun, numGoroutines, providerCount)
		}
	}
}
