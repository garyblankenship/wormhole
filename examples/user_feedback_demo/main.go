package main

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

// Demo showing the improvements based on user feedback
func main() {
	fmt.Println("🎯 Wormhole User Feedback Improvements Demo")
	fmt.Println("==========================================")

	// IMPROVEMENT 1: QuickOpenRouter() - What the user requested
	fmt.Println("\n1. 🚀 QuickOpenRouter() - Simplified Setup")
	fmt.Println("Before: Complex config + WithOpenAICompatible call")
	fmt.Println("After: One line setup!")
	
	// This is what the user wanted to work:
	// client := wormhole.QuickOpenRouter() // Uses OPENROUTER_API_KEY env var
	fmt.Println("   client := wormhole.QuickOpenRouter() // Just this!")

	// IMPROVEMENT 2: Working examples showing actual API
	fmt.Println("\n2. 📖 Fixed Documentation")
	fmt.Println("Before: README showed non-existent WithOpenRouter() method")
	fmt.Println("After: Shows both quick setup AND manual config patterns")

	// IMPROVEMENT 3: Better error reporting
	fmt.Println("\n3. 🔍 Enhanced Error Reporting")
	fmt.Println("Before: Silent failures with err == nil but response.Text == \"\"")
	fmt.Println("After: Detailed errors with HTTP status, URL, response body")

	// Let's simulate what better errors look like (without API key)
	ctx := context.Background()
	
	// This will fail without API key, but now with better error messages
	config := wormhole.Config{
		DefaultProvider: "openrouter",
		Providers: map[string]types.ProviderConfig{
			"openrouter": {
				APIKey:  "invalid-key-for-demo",
				BaseURL: "https://openrouter.ai/api/v1",
			},
		},
	}
	
	client := wormhole.New(config).WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
		APIKey: "invalid-key-for-demo",
	})

	fmt.Println("\n   Testing with invalid API key to show error improvements...")
	response, err := client.Text().
		Model("openai/gpt-5-mini").
		Prompt("test").
		Generate(ctx)

	if err != nil {
		fmt.Printf("✅ Enhanced error (includes HTTP status, URL, response):\n   %v\n", err)
	} else if response.Text == "" {
		fmt.Println("❌ This would have been a silent failure before our fix!")
	}

	fmt.Println("\n4. 🌌 What This Enables:")
	fmt.Println("   - 3-step AI evaluation system (GPT-5-mini → GPT-5-mini → Claude Opus)")
	fmt.Println("   - Multi-model comparison with automatic fallbacks")
	fmt.Println("   - Cost optimization strategies")
	fmt.Println("   - Production-ready error handling")
	fmt.Println("   - No more vendor lock-in!")

	fmt.Println("\n🎉 User feedback implemented! Ready for the multiverse of models!")
}