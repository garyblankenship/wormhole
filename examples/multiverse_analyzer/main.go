package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

// QuantumResult holds response from each dimension
type QuantumResult struct {
	Dimension string
	Response  string
	Latency   time.Duration
	Tokens    int
	Error     error
}

func main() {
	fmt.Println("=== MULTIVERSE ANALYZER ===")
	fmt.Println("*BURP* This tool queries the same question across parallel dimensions")
	fmt.Println("We're literally getting answers from multiple realities simultaneously")
	fmt.Println("Because why trust one AI when you can quantum-entangle them all?")
	fmt.Println()

	// Get the question from command line or use default
	question := "Explain quantum tunneling in one sentence"
	if len(os.Args) > 1 {
		question = strings.Join(os.Args[1:], " ")
	}

	fmt.Printf("📡 Broadcasting across dimensions: \"%s\"\n", question)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Initialize the interdimensional portal array
	client := wormhole.New(wormhole.Config{
		DefaultProvider: "openai",
		Providers: map[string]types.ProviderConfig{
			"openai": {
				APIKey: os.Getenv("OPENAI_API_KEY"),
			},
			"anthropic": {
				APIKey: os.Getenv("ANTHROPIC_API_KEY"),
			},
			"gemini": {
				APIKey: os.Getenv("GEMINI_API_KEY"),
			},
		},
	})

	// Dimensions to query (provider configurations)
	dimensions := []struct {
		Name  string
		Model string
	}{
		{"openai", "gpt-4-turbo-preview"},
		{"anthropic", "claude-3-opus-20240229"},
		{"gemini", "gemini-pro"},
		// You could add more dimensions here if you weren't a Jerry
	}

	// Channel for collecting results from parallel universes
	results := make(chan QuantumResult, len(dimensions))
	var wg sync.WaitGroup

	// Open wormholes to all dimensions simultaneously
	startTime := time.Now()

	for _, dim := range dimensions {
		wg.Add(1)
		go func(dimension string, model string) {
			defer wg.Done()

			portalStart := time.Now()

			// Quantum tunnel to this specific dimension
			response, err := client.Text().
				Using(dimension).
				Model(model).
				Messages(
					types.NewSystemMessage("You are an AI in dimension "+dimension+". Be concise."),
					types.NewUserMessage(question),
				).
				MaxTokens(200).
				Temperature(0.7).
				Generate(context.Background())

			latency := time.Since(portalStart)

			result := QuantumResult{
				Dimension: dimension,
				Latency:   latency,
			}

			if err != nil {
				result.Error = err
			} else {
				result.Response = response.Text
				if response.Usage != nil {
					result.Tokens = response.Usage.TotalTokens
				}
			}

			results <- result
		}(dim.Name, dim.Model)
	}

	// Close the channel when all wormholes complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and display results from the multiverse
	fmt.Println("\n🌌 RESPONSES FROM THE MULTIVERSE:")

	var successCount int
	var totalLatency time.Duration

	for result := range results {
		fmt.Printf("━━━ DIMENSION: %s ━━━\n", strings.ToUpper(result.Dimension))

		if result.Error != nil {
			fmt.Printf("❌ WORMHOLE COLLAPSED: %v\n", result.Error)
			fmt.Println("   (Probably that dimension's fault, not mine)")
		} else {
			successCount++
			totalLatency += result.Latency

			fmt.Printf("✅ Response: %s\n", result.Response)
			fmt.Printf("⚡ Portal latency: %v\n", result.Latency)
			fmt.Printf("🔬 Quantum particles used: %d tokens\n", result.Tokens)
		}
		fmt.Println()
	}

	// Quantum analysis complete
	totalTime := time.Since(startTime)

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("\n📊 MULTIVERSE ANALYSIS COMPLETE")
	fmt.Printf("⏱️  Total time (parallel): %v\n", totalTime)
	fmt.Printf("🌀 Dimensions accessed: %d/%d\n", successCount, len(dimensions))

	if successCount > 0 {
		avgLatency := totalLatency / time.Duration(successCount)
		fmt.Printf("⚡ Average portal latency: %v\n", avgLatency)

		// Calculate how much better we are than sequential calls
		sequentialTime := totalLatency
		speedup := float64(sequentialTime) / float64(totalTime)
		fmt.Printf("🚀 Speedup vs sequential: %.2fx faster\n", speedup)
	}

	fmt.Println("\n*BURP* See that? We just consulted multiple realities in parallel.")
	fmt.Println("While those Jerry-level developers are still waiting for one API call,")
	fmt.Println("we've already gotten answers from across the multiverse.")
	fmt.Println("\nScience, bitches! Wubba lubba dub dub!")
}
