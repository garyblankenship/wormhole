package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	fmt.Println("=== PORTAL STREAM DEMONSTRATOR ===")
	fmt.Println("*BURP* This shows REAL-TIME streaming through interdimensional wormholes")
	fmt.Println("Each token travels through its own quantum micro-tunnel")
	fmt.Println("Watch as we bend spacetime in real-time...")
	fmt.Println()

	// Get prompt from args or use a default that shows streaming
	prompt := "Write a short story about Rick Sanchez discovering a new dimension. Include *BURP* sounds."
	if len(os.Args) > 1 {
		prompt = strings.Join(os.Args[1:], " ")
	}

	// Initialize the quantum streaming apparatus using functional options
	client := wormhole.New(
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
		wormhole.WithAnthropic(os.Getenv("ANTHROPIC_API_KEY")),
	)

	// Choose streaming provider (some are better at streaming than others)
	provider := "openai"
	if os.Getenv("STREAM_PROVIDER") != "" {
		provider = os.Getenv("STREAM_PROVIDER")
	}

	fmt.Printf("📡 Opening streaming wormhole to %s dimension...\n", provider)
	fmt.Printf("📝 Prompt: %s\n", prompt)
	fmt.Println("\n" + strings.Repeat("━", 50))
	fmt.Println("STREAMING RESPONSE:")
	fmt.Println(strings.Repeat("━", 50))

	// Track streaming metrics because SCIENCE
	var tokenCount int
	var firstTokenTime time.Time
	streamStart := time.Now()

	// Open the streaming wormhole
	ctx := context.Background()
	chunks, err := client.Text().
		Using(provider).
		Model(getStreamingModel(provider)).
		Messages(
			types.NewSystemMessage("You are streaming through an interdimensional wormhole at 67 nanoseconds per operation."),
			types.NewUserMessage(prompt),
		).
		MaxTokens(500).
		Temperature(0.8).
		Stream(ctx)

	if err != nil {
		fmt.Printf("\n❌ *BURP* Wormhole failed to open: %v\n", err)
		fmt.Println("Probably because you didn't set up your API keys correctly, Jerry.")
		os.Exit(1)
	}

	// Process the quantum stream
	fmt.Print("\n")
	for chunk := range chunks {
		if chunk.Error != nil {
			fmt.Printf("\n\n❌ Stream disruption: %v\n", chunk.Error)
			break
		}

		// Track first token for TTFT (Time To First Token) metric
		if tokenCount == 0 && chunk.Delta.Content != "" {
			firstTokenTime = time.Now()
		}

		// Display the streaming content with visual indicator
		if chunk.Delta.Content != "" {
			fmt.Print(chunk.Delta.Content)
			tokenCount++

			// Add a subtle streaming effect (optional dramatic pause)
			if tokenCount%10 == 0 {
				time.Sleep(10 * time.Millisecond) // For dramatic effect
			}
		}

		// Check if stream is complete
		if chunk.FinishReason != nil && *chunk.FinishReason == types.FinishReasonStop {
			fmt.Println("\n\n✅ Stream complete - Wormhole closed successfully")
			break
		}
	}

	// Calculate and display quantum metrics
	streamDuration := time.Since(streamStart)
	ttft := firstTokenTime.Sub(streamStart)

	fmt.Println("\n" + strings.Repeat("━", 50))
	fmt.Println("📊 QUANTUM STREAMING METRICS:")
	fmt.Println(strings.Repeat("━", 50))

	fmt.Printf("⚡ Time to First Token (TTFT): %v\n", ttft)
	fmt.Printf("🌀 Total streaming duration: %v\n", streamDuration)
	fmt.Printf("📦 Tokens streamed: %d\n", tokenCount)

	if tokenCount > 0 && streamDuration > 0 {
		tokensPerSecond := float64(tokenCount) / streamDuration.Seconds()
		fmt.Printf("💫 Streaming rate: %.2f tokens/second\n", tokensPerSecond)

		// Calculate theoretical vs actual performance
		theoreticalOps := 10_500_000 // Our 10.5M ops/sec capability
		actualOps := int(tokensPerSecond)
		efficiency := (float64(actualOps) / float64(theoreticalOps)) * 100

		fmt.Printf("🔬 Wormhole efficiency: %.6f%%\n", efficiency)
		fmt.Printf("   (Limited by the provider's dimension, not our portal)\n")
	}

	fmt.Println("\n*BURP* See that? Real-time streaming through quantum tunnels.")
	fmt.Println("Each token traveled through its own micro-wormhole.")
	fmt.Println("That's the difference between science and whatever you were using before.")
	fmt.Println("\nWubba lubba dub dub!")
}

func getStreamingModel(provider string) string {
	switch provider {
	case "openai":
		return "gpt-4o"
	case "anthropic":
		return "claude-3-opus-20240229"
	default:
		return "gpt-3.5-turbo"
	}
}
