package main

import (
	"context"
	"fmt"
	"time"

	"github.com/garyblankenship/wormhole/pkg/middleware"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func main() {
	fmt.Println("=== Enhanced Metrics Example ===")

	// Create enhanced metrics collector with default configuration
	collector := middleware.NewEnhancedMetricsCollector(nil)
	fmt.Println("Created enhanced metrics collector")

	// Example 1: Record requests with labels
	labels1 := &middleware.RequestLabels{
		Provider: "openai",
		Model:    "gpt-4",
		Method:   "text",
		ErrorType: "",
	}

	labels2 := &middleware.RequestLabels{
		Provider: "anthropic",
		Model:    "claude-3",
		Method:   "stream",
		ErrorType: "",
	}

	// Record successful request
	collector.RecordRequest(labels1, 150*time.Millisecond, nil, 0, 100, 250)
	fmt.Println("Recorded successful request for OpenAI GPT-4")

	// Record failed request
	collector.RecordRequest(labels2, 75*time.Millisecond, fmt.Errorf("rate limit exceeded"), 2, 50, 0)
	fmt.Println("Recorded failed request for Anthropic Claude-3")

	// Get statistics
	stats1 := collector.GetStats(labels1)
	fmt.Printf("\nOpenAI GPT-4 Stats:\n")
	fmt.Printf("  Requests: %d\n", stats1["requests"])
	fmt.Printf("  Errors: %d\n", stats1["errors"])
	fmt.Printf("  Input Tokens: %d\n", stats1["input_tokens"])
	fmt.Printf("  Output Tokens: %d\n", stats1["output_tokens"])

	// Export to Prometheus format
	prometheusOutput := collector.PrometheusExporter()
	fmt.Printf("\nPrometheus Export (first 500 chars):\n%s\n", prometheusOutput[:min(500, len(prometheusOutput))])

	// Export to JSON format
	jsonOutput := collector.JSONExporter()
	fmt.Printf("\nJSON Export Global Stats:\n")
	globalStats := jsonOutput["global"].(map[string]interface{})
	fmt.Printf("  Total Requests: %v\n", globalStats["requests"])
	fmt.Printf("  Total Errors: %v\n", globalStats["errors"])

	// Example 2: Create type-safe middleware
	fmt.Println("\n=== Type-Safe Middleware Example ===")

	// Create typed middleware
	typedMiddleware := middleware.NewTypedEnhancedMetricsMiddleware(collector)
	fmt.Println("Created typed enhanced metrics middleware")

	// Example handler
	textHandler := func(ctx context.Context, req types.TextRequest) (*types.TextResponse, error) {
		// Simulate processing
		time.Sleep(10 * time.Millisecond)
		return &types.TextResponse{
			Text: "Hello, world!",
		}, nil
	}

	// Wrap handler with middleware
	wrappedHandler := typedMiddleware.ApplyText(textHandler)

	// Create a context with provider information
	ctx := context.WithValue(context.Background(), "wormhole_provider", "google")

	// Create request
	request := types.TextRequest{
		BaseRequest: types.BaseRequest{
			Model: "gemini-pro",
		},
		Messages: []types.Message{
			types.BaseMessage{
				Role:    types.RoleUser,
				Content: "Hello",
			},
		},
	}

	// Execute handler
	resp, err := wrappedHandler(ctx, request)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Response: %s\n", resp.Text)
	}

	// Get updated stats
	allStats := collector.GetAllStats()
	fmt.Printf("\nFinal Global Stats:\n")
	finalGlobalStats := allStats["global"].(map[string]interface{})
	fmt.Printf("  Total Requests: %v\n", finalGlobalStats["requests"])
	fmt.Printf("  Total Errors: %v\n", finalGlobalStats["errors"])

	fmt.Println("\n=== Example Complete ===")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}