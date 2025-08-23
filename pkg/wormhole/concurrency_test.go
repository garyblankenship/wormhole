package wormhole

import (
	"errors"
	"sync"
	"testing"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// TestConcurrentProviderAccess tests that multiple goroutines can safely
// access the Provider method simultaneously without causing data races
func TestConcurrentProviderAccess(t *testing.T) {
	// Create wormhole with OpenAI provider configured
	w := New(
		WithDefaultProvider("openai"),
		WithOpenAI("test-key"),
	)

	const numGoroutines = 100
	const numIterations = 10

	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines*numIterations)

	// Launch multiple goroutines that simultaneously call Provider
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				provider, err := w.Provider("openai")
				if err != nil {
					errChan <- err
					return
				}
				if provider == nil {
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
		go func(routineID int) {
			defer wg.Done()

			// This simulates the exact call pattern that caused the bug
			builder := w.Text().Model("gpt-4").Prompt("You are a helpful assistant")
			if builder == nil {
				errChan <- errors.New("text builder is nil")
				return
			}

			// Try to get the provider (this is where the race condition occurred)
			provider, err := w.getProvider("")
			if err != nil {
				errChan <- err
				return
			}
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
			provider, err := w.Provider("openai")
			if err != nil {
				errChan <- err
				return
			}
			if provider == nil {
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

				provider, err := w.Provider("openai")
				if err != nil {
					errChan <- err
					return
				}
				providerChan <- provider
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
