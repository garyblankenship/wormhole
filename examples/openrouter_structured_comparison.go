package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY environment variable is required")
	}

	w := wormhole.New(
		wormhole.WithDefaultProvider("openrouter"),
		wormhole.WithOpenAICompatible("openrouter", "https://openrouter.ai/api/v1", types.ProviderConfig{
			APIKey: apiKey,
		}),
		wormhole.WithTimeout(2*time.Minute),
	)

	ctx := context.Background()
	prompt := "Analyze this text: 'I absolutely love this new smartphone! The camera quality is amazing and the battery lasts all day.'"

	// Shared schema for both approaches
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"sentiment": map[string]interface{}{
				"type": "string",
				"enum": []string{"positive", "negative", "neutral"},
			},
			"confidence": map[string]interface{}{
				"type":    "number",
				"minimum": 0,
				"maximum": 1,
			},
			"key_aspects": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
		},
		"required": []string{"sentiment", "confidence"},
	}

	fmt.Println("🌌 OpenRouter Structured Output Comparison")
	fmt.Println("=========================================")

	// Approach 1: Wormhole Structured (Works with ALL models)
	fmt.Println("\n1. 🔧 Wormhole Structured (Tool-Based) - Claude Opus")
	claudeStart := time.Now()
	claudeResponse, err := w.Structured().
		Model("anthropic/claude-opus-4.1").
		Prompt(prompt).
		Schema(schema).
		SchemaName("sentiment_analysis").
		MaxTokens(300).
		Temperature(0.1).
		Generate(ctx)

	claudeDuration := time.Since(claudeStart)

	if err != nil {
		log.Printf("❌ Claude error: %v", err)
	} else {
		fmt.Printf("✅ Claude Opus Response (%v):\n", claudeDuration)
		prettyJSON, _ := json.MarshalIndent(claudeResponse.Data, "", "  ")
		fmt.Printf("%s\n", string(prettyJSON))
	}

	// Approach 2: Wormhole Structured with OpenAI (for comparison)
	fmt.Println("\n2. 🔧 Wormhole Structured (Tool-Based) - OpenAI")
	openaiStructuredStart := time.Now()
	openaiStructuredResponse, err := w.Structured().
		Model("openai/gpt-4o-mini").
		Prompt(prompt).
		Schema(schema).
		SchemaName("sentiment_analysis").
		MaxTokens(300).
		Temperature(0.1).
		Generate(ctx)

	openaiStructuredDuration := time.Since(openaiStructuredStart)

	if err != nil {
		log.Printf("❌ OpenAI Structured error: %v", err)
	} else {
		fmt.Printf("✅ OpenAI Structured Response (%v):\n", openaiStructuredDuration)
		prettyJSON, _ := json.MarshalIndent(openaiStructuredResponse.Data, "", "  ")
		fmt.Printf("%s\n", string(prettyJSON))
	}

	// Approach 3: OpenRouter Native (OpenAI models only)
	fmt.Println("\n3. 🏠 OpenRouter Native (response_format) - OpenAI Only")
	nativeStart := time.Now()
	nativeResponse, err := w.Text().
		Model("openai/gpt-4o-mini").
		Messages(types.NewUserMessage(prompt)).
		ProviderOptions(map[string]interface{}{
			"response_format": map[string]interface{}{
				"type": "json_schema",
				"json_schema": map[string]interface{}{
					"name":   "sentiment_analysis",
					"strict": true,
					"schema": schema,
				},
			},
		}).
		MaxTokens(300).
		Temperature(0.1).
		Generate(ctx)

	nativeDuration := time.Since(nativeStart)

	if err != nil {
		log.Printf("❌ Native response_format error: %v", err)
	} else {
		fmt.Printf("✅ OpenRouter Native Response (%v):\n", nativeDuration)
		// Parse the JSON manually since it returns TextResponse
		var parsedData map[string]interface{}
		if err := json.Unmarshal([]byte(nativeResponse.Text), &parsedData); err != nil {
			log.Printf("❌ Failed to parse native JSON: %v", err)
		} else {
			prettyJSON, _ := json.MarshalIndent(parsedData, "", "  ")
			fmt.Printf("%s\n", string(prettyJSON))
		}
	}

	// Approach 4: Test Claude with OpenRouter Native (will fail)
	fmt.Println("\n4. ❌ OpenRouter Native with Claude (Expected to Fail)")
	claudeNativeStart := time.Now()
	claudeNativeResponse, err := w.Text().
		Model("anthropic/claude-3.5-sonnet").
		Messages(types.NewUserMessage(prompt)).
		ProviderOptions(map[string]interface{}{
			"response_format": map[string]interface{}{
				"type": "json_schema",
				"json_schema": map[string]interface{}{
					"name":   "sentiment_analysis",
					"strict": true,
					"schema": schema,
				},
			},
		}).
		MaxTokens(300).
		Temperature(0.1).
		Generate(ctx)

	claudeNativeDuration := time.Since(claudeNativeStart)

	if err != nil {
		fmt.Printf("❌ Expected failure (%v): %v\n", claudeNativeDuration, err)
		fmt.Println("   💡 This is why you should use wormhole's .Structured() for Claude!")
	} else {
		fmt.Printf("🤔 Unexpected success (%v): %s\n", claudeNativeDuration, claudeNativeResponse.Text)
		fmt.Println("   Note: Claude might ignore response_format and return unstructured text")
	}

	// Summary
	fmt.Println("\n📊 Summary & Recommendations")
	fmt.Println("============================")
	fmt.Println("✅ Use wormhole.Structured() for:")
	fmt.Println("   • ALL Claude models (anthropic/*)")
	fmt.Println("   • Consistent behavior across providers")
	fmt.Println("   • Automatic parsing to StructuredResponse")
	fmt.Println("   • Enhanced error handling")
	fmt.Println()
	fmt.Println("⚡ Use OpenRouter native for:")
	fmt.Println("   • OpenAI models only (openai/*)")
	fmt.Println("   • When you need OpenRouter's native validation")
	fmt.Println("   • Simple extraction tasks")
	fmt.Println()
	fmt.Println("🚨 Limitations:")
	fmt.Println("   • OpenRouter native doesn't support Claude models")
	fmt.Println("   • Native approach requires manual JSON parsing")
	fmt.Println("   • Less consistent error handling")
}