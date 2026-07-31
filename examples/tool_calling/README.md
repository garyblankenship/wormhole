# Tool Calling Example

This example demonstrates the native tool use / function calling feature in Wormhole.

## What This Example Shows

1. **Tool Registration** - How to register Go functions as tools the AI can call
2. **Automatic Execution** - How tools are automatically executed when the model requests them
3. **Multi-Turn Conversations** - How the SDK handles back-and-forth between model and tools
4. **Multiple Tools** - Using several tools in a single conversation
5. **Manual Mode** - How to own handlers and complete the tool-result continuation yourself

## Running the Example

### Prerequisites

1. Set your OpenAI API key:
```bash
export OPENAI_API_KEY="sk-..."
```

2. Ensure dependencies are installed:
```bash
go mod download
```

### Run

```bash
cd examples/tool_calling
go run main.go
```

## Expected Output

```
✓ Registered 3 tools

=== Example 1: Weather Query ===
🔧 Executing get_weather(city=San Francisco, unit=fahrenheit)

📝 AI Response: The current weather in San Francisco is 72°F and sunny with 65% humidity.

=== Example 2: Multi-Tool Conversation ===
🔧 Executing get_weather(city=London, unit=fahrenheit)
🔧 Executing calculate(expression=25 + 17)

📝 AI Response: The weather in London is 55°F and cloudy. And 25 + 17 equals 42.

=== Example 3: Manual Tool Execution ===
🔧 Model requested 1 tool call(s):
  - get_current_time with args: map[timezone:Asia/Tokyo]

🔧 Executing get_current_time(timezone=Asia/Tokyo)

📝 AI Response: It is currently ...
```

## How It Works

### 1. Tool Registration

Tools are registered at the client level with:
- **Name**: Unique identifier for the tool
- **Description**: Helps the AI decide when to use it
- **Schema**: JSON Schema for input validation
- **Handler**: Go function that executes the tool

```go
client.RegisterTool(
    "get_weather",
    "Get the current weather for a given city",
    map[string]any{
        "type": "object",
        "properties": map[string]any{
            "city": map[string]any{"type": "string"},
            "unit": map[string]any{"type": "string", "enum": []string{"celsius", "fahrenheit"}},
        },
        "required": []string{"city"},
    },
    func(ctx context.Context, args map[string]any) (any, error) {
        city := args["city"].(string)
        // ... fetch weather ...
        return map[string]any{"temp": 72, "condition": "sunny"}, nil
    },
)
```

### 2. Automatic Execution (Default)

When you call `.Generate()` with registered tools:

1. SDK sends request to model with available tools
2. Model returns tool calls (if needed)
3. **SDK automatically executes tools**
4. SDK sends results back to model
5. Model generates final response
6. Repeat until no more tool calls (or max iterations)

```go
response, err := client.Text().
    Prompt("What's the weather in SF?").
    Generate(ctx)  // Automatic tool execution!
```

### 3. Manual Execution (Opt-In)

Keep a caller-owned handler map, explicitly send the registered tool schemas,
execute the requested handlers, and continue with assistant and tool-result
messages:

```go
handlers := map[string]types.ToolHandler{
    "get_weather": getWeather,
}
tools := client.ListTools()
prompt := "What's the weather in SF?"

response, err := client.Text().
    Model("gpt-5.6").
    Prompt(prompt).
    Tools(tools...).
    WithToolsDisabled().
    Generate(ctx)
if err != nil {
    return err
}

normalizedCalls := make([]types.ToolCall, 0, len(response.ToolCalls))
for _, call := range response.ToolCalls {
    normalized, err := types.NormalizeToolCall(call)
    if err != nil {
        return err
    }
    normalizedCalls = append(normalizedCalls, normalized)
}

assistant := types.NewAssistantMessage(response.Text)
assistant.ToolCalls = normalizedCalls
assistant.Thinking = response.Thinking
messages := []types.Message{types.NewUserMessage(prompt), assistant}

for _, call := range normalizedCalls {
    handler, ok := handlers[call.Name]
    if !ok {
        return fmt.Errorf("tool %q is not admitted", call.Name)
    }
    result, err := handler(ctx, call.Arguments)
    if err != nil {
        return err
    }
    content, err := json.Marshal(result)
    if err != nil {
        return err
    }
    message := types.NewToolResultMessage(call.ID, string(content))
    message.FunctionName = call.Name
    messages = append(messages, message)
}

continued, err := client.Text().
    Model("gpt-5.6").
    Messages(messages...).
    Tools(tools...).
    WithToolsDisabled().
    Generate(ctx)
```

Manual mode bypasses the SDK executor. The caller owns schema validation,
admission limits, execution timeouts, and retries, in addition to deciding
which handlers are allowed.

## Tool Handler Signature

All tool handlers must follow this signature:

```go
func(ctx context.Context, args map[string]any) (result any, err error)
```

- **ctx**: Context for cancellation and timeouts
- **args**: Tool arguments as a map (validated against schema)
- **result**: Any JSON-serializable return value
- **err**: Error if execution failed (sent back to model)

## Configuration Options

```go
client.Text().
    WithToolsEnabled().           // Enable auto-execution (default if tools registered)
    WithToolsDisabled().          // Disable auto-execution (manual mode)
    WithMaxToolIterations(10).    // Set max rounds (default: 10)
    Generate(ctx)
```

## Best Practices

1. **Clear Descriptions**: Help the AI understand when to use each tool
2. **Schema Validation**: Define proper JSON schemas for type safety
3. **Error Handling**: Return descriptive errors - they're sent to the model
4. **Idempotency**: Tools may be called multiple times
5. **Timeouts**: Use context for long-running operations
6. **Security**: Validate and sanitize all inputs; in manual mode, also enforce
   admission, timeouts, and retry policy before invoking caller-owned handlers

## Advanced Usage

### Error Recovery

Tool errors are sent back to the model, allowing it to retry or adjust:

```go
func riskyTool(ctx context.Context, args map[string]any) (any, error) {
    result, err := doSomethingRisky()
    if err != nil {
        // Model receives this error and can retry with different params
        return nil, fmt.Errorf("operation failed: %w", err)
    }
    return result, nil
}
```

### Context-Aware Tools

Use context for cancellation and timeouts:

```go
func slowTool(ctx context.Context, args map[string]any) (any, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    case result := <-doExpensiveWork(args):
        return result, nil
    }
}
```

### Multi-Step Reasoning

The model can call multiple tools in sequence:

```
User: "What's the weather in the city where it's 3pm right now?"
  → Model calls get_current_time(timezone="America/New_York")
  → Returns "New York"
  → Model calls get_weather(city="New York")
  → Returns weather data
  → Model responds: "It's 72°F in New York where it's currently 3pm"
```

## Troubleshooting

### Tools Not Being Called

- Check tool descriptions are clear
- Verify schema matches expected input
- Ensure the selected model supports function calling; the current examples use
  gpt-5.6, claude-sonnet-5, and gemini-3.6-flash.

### Infinite Loops

- Set `WithMaxToolIterations()` to prevent runaway execution
- Review tool outputs - ensure they provide useful data
- Check for circular tool dependencies

### Type Errors

- Tool arguments are `map[string]any` - use type assertions
- Return JSON-serializable types only
- Use schema validation to enforce types

## See Also

- [Main README](../../README.md) - Full Wormhole documentation
- [Package Reference](https://pkg.go.dev/github.com/garyblankenship/wormhole/v2) - Exported API
