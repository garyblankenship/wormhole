package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/types"
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
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("Close error: %v\n", err)
		}
	}()

	// Register tools that the AI can call
	handlers := registerTools(client)

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
	runManualToolExample(client, handlers)
}

// registerTools registers all available tools with the client
func registerTools(client *wormhole.Wormhole) map[string]types.ToolHandler {
	handlers := map[string]types.ToolHandler{
		"get_weather":      getWeather,
		"calculate":        calculate,
		"get_current_time": getCurrentTime,
	}

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
		handlers["get_weather"],
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
		handlers["calculate"],
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
		handlers["get_current_time"],
	)

	fmt.Printf("✓ Registered %d tools\n\n", client.ToolCount())
	return handlers
}

// getWeather simulates fetching weather data
func getWeather(ctx context.Context, args map[string]any) (any, error) {
	city, ok := args["city"].(string)
	if !ok || city == "" {
		return nil, fmt.Errorf("city must be a non-empty string")
	}
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
	expression, ok := args["expression"].(string)
	if !ok || expression == "" {
		return nil, fmt.Errorf("expression must be a non-empty string")
	}

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
	timezone, ok := args["timezone"].(string)
	if !ok || timezone == "" {
		return nil, fmt.Errorf("timezone must be a non-empty string")
	}

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
		Model("gpt-5.6").
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
		Model("gpt-5.6").
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
func runManualToolExample(client *wormhole.Wormhole, handlers map[string]types.ToolHandler) {
	ctx := context.Background()
	prompt := "What time is it in Tokyo?"
	tools := client.ListTools()

	response, err := client.Text().
		Model("gpt-5.6").
		Prompt(prompt).
		Tools(tools...).
		WithToolsDisabled(). // Disable automatic execution
		Generate(ctx)

	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	if len(response.ToolCalls) == 0 {
		fmt.Printf("\n📝 AI Response (no tools needed): %s\n", response.Text)
		return
	}

	fmt.Printf("\n🔧 Model requested %d tool call(s):\n", len(response.ToolCalls))
	for _, toolCall := range response.ToolCalls {
		fmt.Printf("  - %s with args: %v\n", toolCall.Name, toolCall.Arguments)
	}

	continued, err := continueManualToolCalls(ctx, client, handlers, prompt, response)
	if err != nil {
		log.Printf("Continuation error: %v\n", err)
		return
	}

	fmt.Printf("\n📝 AI Response: %s\n", continued.Text)
}

func continueManualToolCalls(
	ctx context.Context,
	client *wormhole.Wormhole,
	handlers map[string]types.ToolHandler,
	prompt string,
	response *types.TextResponse,
) (*types.TextResponse, error) {
	normalizedCalls := make([]types.ToolCall, 0, len(response.ToolCalls))
	for _, toolCall := range response.ToolCalls {
		normalized, err := types.NormalizeToolCall(toolCall)
		if err != nil {
			return nil, fmt.Errorf("normalize tool call: %w", err)
		}
		normalizedCalls = append(normalizedCalls, normalized)
	}

	assistant := types.NewAssistantMessage(response.Text)
	assistant.ToolCalls = normalizedCalls
	assistant.Thinking = response.Thinking
	messages := []types.Message{types.NewUserMessage(prompt), assistant}
	for _, toolCall := range normalizedCalls {
		messages = append(messages, executeManualTool(ctx, handlers, toolCall))
	}

	return client.Text().
		Model("gpt-5.6").
		Messages(messages...).
		Tools(client.ListTools()...).
		WithToolsDisabled().
		Generate(ctx)
}

func executeManualTool(ctx context.Context, handlers map[string]types.ToolHandler, toolCall types.ToolCall) *types.ToolResultMessage {
	normalized, err := types.NormalizeToolCall(toolCall)
	if err != nil {
		return manualToolResultMessage(toolCall, err.Error(), err.Error())
	}
	toolCall = normalized

	handler, ok := handlers[toolCall.Name]
	if !ok {
		message := fmt.Sprintf("tool %q is not admitted", toolCall.Name)
		return manualToolResultMessage(toolCall, message, message)
	}

	result, err := handler(ctx, toolCall.Arguments)
	if err != nil {
		return manualToolResultMessage(toolCall, err.Error(), err.Error())
	}
	content, err := json.Marshal(result)
	if err != nil {
		message := fmt.Sprintf("serialize %q result: %v", toolCall.Name, err)
		return manualToolResultMessage(toolCall, message, message)
	}
	return manualToolResultMessage(toolCall, string(content), "")
}

func manualToolResultMessage(toolCall types.ToolCall, content, errorMessage string) *types.ToolResultMessage {
	message := types.NewToolResultMessage(toolCall.ID, content)
	message.FunctionName = toolCall.Name
	if errorMessage != "" {
		message.WithError(errorMessage)
	}
	return message
}
