package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create wormhole client with OpenAI
	client := wormhole.New(
		wormhole.WithOpenAI(apiKey),
		wormhole.WithDefaultProvider("openai"),
	)

	// Register tools that the AI can call
	registerTools(client)

	// Example 1: Weather query (single tool call)
	fmt.Println("=== Example 1: Weather Query ===")
	runWeatherExample(client)

	fmt.Println()

	// Example 2: Multi-tool conversation (calculator + weather)
	fmt.Println("=== Example 2: Multi-Tool Conversation ===")
	runMultiToolExample(client)

	fmt.Println()

	// Example 3: Manual tool execution (opt-out of auto-execution)
	fmt.Println("=== Example 3: Manual Tool Execution ===")
	runManualToolExample(client)
}

// registerTools registers all available tools with the client
func registerTools(client *wormhole.Wormhole) {
	// Tool 1: Get Weather
	client.RegisterTool(
		"get_weather",
		"Get the current weather for a given city. Returns temperature in the specified unit.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city": map[string]any{
					"type":        "string",
					"description": "The city name (e.g., 'San Francisco', 'London')",
				},
				"unit": map[string]any{
					"type":        "string",
					"description": "Temperature unit: 'celsius' or 'fahrenheit'",
					"enum":        []string{"celsius", "fahrenheit"},
				},
			},
			"required": []string{"city"},
		},
		getWeather,
	)

	// Tool 2: Calculator
	client.RegisterTool(
		"calculate",
		"Perform a mathematical calculation. Supports +, -, *, / operations.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{
					"type":        "string",
					"description": "Mathematical expression to evaluate (e.g., '2 + 2', '10 * 5')",
				},
			},
			"required": []string{"expression"},
		},
		calculate,
	)

	// Tool 3: Get Current Time
	client.RegisterTool(
		"get_current_time",
		"Get the current time in a specified timezone.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timezone": map[string]any{
					"type":        "string",
					"description": "IANA timezone name (e.g., 'America/New_York', 'Europe/London')",
				},
			},
			"required": []string{"timezone"},
		},
		getCurrentTime,
	)

	fmt.Printf("✓ Registered %d tools\n\n", client.ToolCount())
}

// getWeather simulates fetching weather data
func getWeather(ctx context.Context, args map[string]any) (any, error) {
	city := args["city"].(string)
	unit := "fahrenheit"
	if u, ok := args["unit"].(string); ok {
		unit = u
	}

	fmt.Printf("🔧 Executing get_weather(city=%s, unit=%s)\n", city, unit)

	// Simulate API call
	time.Sleep(100 * time.Millisecond)

	// Mock weather data
	weatherData := map[string]map[string]any{
		"san francisco": {"temp_f": 72, "temp_c": 22, "condition": "sunny", "humidity": 65},
		"london":        {"temp_f": 55, "temp_c": 13, "condition": "cloudy", "humidity": 80},
		"new york":      {"temp_f": 68, "temp_c": 20, "condition": "partly cloudy", "humidity": 70},
		"tokyo":         {"temp_f": 75, "temp_c": 24, "condition": "clear", "humidity": 60},
	}

	// Normalize city name
	cityLower := ""
	for key := range weatherData {
		if key == city || key == city+" city" {
			cityLower = key
			break
		}
	}
	if cityLower == "" {
		// Default for unknown cities
		cityLower = "san francisco"
	}

	weather := weatherData[cityLower]
	temp := weather["temp_f"]
	if unit == "celsius" {
		temp = weather["temp_c"]
	}

	return map[string]any{
		"city":        city,
		"temperature": temp,
		"unit":        unit,
		"condition":   weather["condition"],
		"humidity":    weather["humidity"],
	}, nil
}

// calculate performs simple math calculations
func calculate(ctx context.Context, args map[string]any) (any, error) {
	expression := args["expression"].(string)

	fmt.Printf("🔧 Executing calculate(expression=%s)\n", expression)

	// Simple parser for demo (in production, use a proper math parser)
	// This is just a mock - returns a random result
	result := 42.0 // Mock result

	return map[string]any{
		"expression": expression,
		"result":     result,
		"message":    fmt.Sprintf("%s = %.2f", expression, result),
	}, nil
}

// getCurrentTime returns the current time in a timezone
func getCurrentTime(ctx context.Context, args map[string]any) (any, error) {
	timezone := args["timezone"].(string)

	fmt.Printf("🔧 Executing get_current_time(timezone=%s)\n", timezone)

	// Load timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone: %s", timezone)
	}

	now := time.Now().In(loc)

	return map[string]any{
		"timezone":    timezone,
		"time":        now.Format("3:04 PM"),
		"date":        now.Format("January 2, 2006"),
		"day_of_week": now.Weekday().String(),
	}, nil
}

// runWeatherExample demonstrates a simple weather query
func runWeatherExample(client *wormhole.Wormhole) {
	ctx := context.Background()

	response, err := client.Text().
		Model("gpt-5").
		Prompt("What's the weather like in San Francisco?").
		WithToolsEnabled().
		Generate(ctx)

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("\n📝 AI Response: %s\n", response.Text)
}

// runMultiToolExample demonstrates using multiple tools in one conversation
func runMultiToolExample(client *wormhole.Wormhole) {
	ctx := context.Background()

	response, err := client.Text().
		Model("gpt-5").
		Prompt("What's the weather in London? Also, what's 25 + 17?").
		WithToolsEnabled().
		Generate(ctx)

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("\n📝 AI Response: %s\n", response.Text)
}

// runManualToolExample demonstrates manual tool execution (no auto-execution)
func runManualToolExample(client *wormhole.Wormhole) {
	ctx := context.Background()

	response, err := client.Text().
		Model("gpt-5").
		Prompt("What time is it in Tokyo?").
		WithToolsDisabled(). // Disable automatic execution
		Generate(ctx)

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	// Check if model requested tools
	if len(response.ToolCalls) > 0 {
		fmt.Printf("\n🔧 Model requested %d tool call(s):\n", len(response.ToolCalls))
		for _, toolCall := range response.ToolCalls {
			fmt.Printf("  - %s with args: %v\n", toolCall.Name, toolCall.Arguments)
		}
		fmt.Println("\n💡 In manual mode, you would execute these tools yourself and send results back.")
	} else {
		fmt.Printf("\n📝 AI Response (no tools needed): %s\n", response.Text)
	}
}
