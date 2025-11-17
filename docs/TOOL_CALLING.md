# Native Tool Use / Function Calling

## Overview

Wormhole now supports **native tool calling** (also known as function calling), allowing you to register Go functions that LLMs can automatically discover and execute. This enables building powerful AI agents that can interact with external systems, databases, APIs, and more.

## Key Features

- ✅ **Automatic Execution** - Tools are executed automatically when the model requests them
- ✅ **Multi-Turn Conversations** - SDK handles the back-and-forth between model and tools seamlessly
- ✅ **Parallel Tool Execution** - Multiple tools are executed concurrently for performance
- ✅ **Error Recovery** - Tool errors are sent back to the model for intelligent retry/adjustment
- ✅ **Thread-Safe Registry** - Register tools from anywhere in your application
- ✅ **Manual Mode** - Opt-out of automatic execution when you need fine-grained control
- ✅ **Infinite Loop Protection** - Max iteration limits prevent runaway tool execution

## Quick Start

### 1. Register Tools

```go
import (
    "context"
    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

client := wormhole.New(wormhole.WithOpenAI("your-api-key"))

// Register a tool with schema and handler
client.RegisterTool(
    "get_weather",                          // Tool name
    "Get current weather for a city",       // Description for the AI
    map[string]any{                         // JSON Schema for validation
        "type": "object",
        "properties": map[string]any{
            "city": map[string]any{
                "type":        "string",
                "description": "City name",
            },
            "unit": map[string]any{
                "type": "string",
                "enum": []string{"celsius", "fahrenheit"},
            },
        },
        "required": []string{"city"},
    },
    func(ctx context.Context, args map[string]any) (any, error) {
        city := args["city"].(string)
        // ... fetch actual weather data ...
        return map[string]any{
            "temperature": 72,
            "condition":   "sunny",
            "humidity":    65,
        }, nil
    },
)
```

### 2. Use Normally - Automatic Execution!

```go
response, err := client.Text().
    Prompt("What's the weather in San Francisco?").
    Generate(context.Background())

fmt.Println(response.Text)
// Output: "The weather in San Francisco is 72°F and sunny with 65% humidity."
```

**What Happens Behind the Scenes:**

1. SDK sends request to model with available tools
2. Model decides to call `get_weather(city="San Francisco")`
3. **SDK automatically executes the tool**
4. SDK sends the result back to the model
5. Model generates the final natural language response
6. You get the complete answer

## Architecture

### Components

```
┌─────────────────────────────────────────────────────────────┐
│                      Wormhole Client                         │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Tool Registry (Thread-Safe)              │   │
│  │  - get_weather: ToolDefinition{schema, handler}      │   │
│  │  - calculate: ToolDefinition{schema, handler}        │   │
│  │  - search_db: ToolDefinition{schema, handler}        │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    Tool Executor                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  1. Execute(toolCall) → runs handler                 │   │
│  │  2. ExecuteAll(toolCalls) → parallel execution       │   │
│  │  3. ExecuteWithTools() → multi-turn orchestration    │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                Multi-Turn Conversation Flow                  │
│                                                              │
│  User Prompt                                                 │
│      ↓                                                       │
│  LLM Request (with tools)                                    │
│      ↓                                                       │
│  Tool Calls Returned? ─── No ──→ Final Response             │
│      │                                                       │
│     Yes                                                      │
│      ↓                                                       │
│  Execute Tools (parallel)                                    │
│      ↓                                                       │
│  Send Results to LLM                                         │
│      ↓                                                       │
│  (Repeat until no more tool calls or max iterations)         │
│      ↓                                                       │
│  Final Response                                              │
└─────────────────────────────────────────────────────────────┘
```

### Tool Handler Signature

All tool handlers must implement this signature:

```go
type ToolHandler func(ctx context.Context, arguments map[string]any) (result any, err error)
```

- **ctx**: Context for cancellation and timeouts
- **arguments**: Tool arguments as a map (keys match your schema)
- **result**: Any JSON-serializable value (maps, structs, primitives)
- **err**: Error if execution failed (sent back to model as error message)

## Configuration Options

### Enable/Disable Automatic Execution

```go
// Automatic execution (default when tools are registered)
response, _ := client.Text().
    Prompt("What's the weather?").
    WithToolsEnabled().  // Explicit opt-in
    Generate(ctx)

// Manual execution (you handle tool calls yourself)
response, _ := client.Text().
    Prompt("What's the weather?").
    WithToolsDisabled().  // Opt-out
    Generate(ctx)

// Check what the model wanted to call
for _, toolCall := range response.ToolCalls {
    fmt.Printf("Model wants: %s(%v)\n", toolCall.Name, toolCall.Arguments)
}
```

### Max Iteration Limits

Prevent infinite loops by setting a maximum number of tool execution rounds:

```go
response, _ := client.Text().
    WithMaxToolIterations(5).  // Default is 10
    Generate(ctx)
```

## Tool Registry API

### Register Tools

```go
client.RegisterTool(name string, description string, schema Schema, handler ToolHandler)
```

### Manage Tools

```go
// Check if tool exists
if client.HasTool("get_weather") {
    // ...
}

// List all registered tools
tools := client.ListTools()
fmt.Printf("Registered: %d tools\n", len(tools))

// Remove a tool
err := client.UnregisterTool("get_weather")

// Remove all tools
client.ClearTools()

// Get count
count := client.ToolCount()
```

## Examples

### Example 1: Weather Tool

```go
client.RegisterTool(
    "get_weather",
    "Get current weather for a city",
    map[string]any{
        "type": "object",
        "properties": map[string]any{
            "city": map[string]any{"type": "string"},
            "unit": map[string]any{
                "type": "string",
                "enum": []string{"celsius", "fahrenheit"},
            },
        },
        "required": []string{"city"},
    },
    func(ctx context.Context, args map[string]any) (any, error) {
        city := args["city"].(string)
        unit := "fahrenheit"
        if u, ok := args["unit"].(string); ok {
            unit = u
        }

        // Fetch from weather API...
        return map[string]any{
            "city":        city,
            "temperature": 72,
            "unit":        unit,
            "condition":   "sunny",
        }, nil
    },
)

response, _ := client.Text().
    Prompt("What's the weather in NYC?").
    Generate(ctx)
```

### Example 2: Database Query Tool

```go
client.RegisterTool(
    "search_users",
    "Search for users in the database",
    map[string]any{
        "type": "object",
        "properties": map[string]any{
            "query": map[string]any{
                "type":        "string",
                "description": "Search query",
            },
            "limit": map[string]any{
                "type":    "integer",
                "minimum": 1,
                "maximum": 100,
            },
        },
        "required": []string{"query"},
    },
    func(ctx context.Context, args map[string]any) (any, error) {
        query := args["query"].(string)
        limit := 10
        if l, ok := args["limit"].(float64); ok {
            limit = int(l)
        }

        // Query your database...
        users, err := db.SearchUsers(ctx, query, limit)
        if err != nil {
            return nil, fmt.Errorf("database error: %w", err)
        }

        return users, nil
    },
)

response, _ := client.Text().
    Prompt("Find users named John").
    Generate(ctx)
```

### Example 3: Multi-Tool Workflow

```go
// Register multiple tools
client.RegisterTool("get_weather", ...)
client.RegisterTool("calculate", ...)
client.RegisterTool("get_current_time", ...)

// The AI can use multiple tools in one conversation
response, _ := client.Text().
    Prompt("What's the weather in London? Also calculate 25 + 17").
    Generate(ctx)

// The SDK automatically:
// 1. Calls get_weather(city="London")
// 2. Calls calculate(expression="25 + 17")
// 3. Sends both results to the model
// 4. Model generates final response using both results
```

## Best Practices

### 1. Clear Tool Descriptions

Help the AI understand when to use each tool:

```go
// ❌ Bad: Vague description
"Get weather"

// ✅ Good: Clear, specific description
"Get the current weather conditions for a given city. Returns temperature, conditions, and humidity."
```

### 2. Comprehensive Schemas

Define proper JSON schemas with descriptions and validation:

```go
map[string]any{
    "type": "object",
    "properties": map[string]any{
        "city": map[string]any{
            "type":        "string",
            "description": "The city name (e.g., 'San Francisco', 'London')",
        },
        "unit": map[string]any{
            "type":        "string",
            "description": "Temperature unit",
            "enum":        []string{"celsius", "fahrenheit"},
        },
    },
    "required": []string{"city"},
}
```

### 3. Descriptive Error Messages

Return errors that help the model understand what went wrong:

```go
func searchTool(ctx context.Context, args map[string]any) (any, error) {
    query := args["query"].(string)

    if len(query) < 3 {
        // ✅ Descriptive error the model can understand
        return nil, fmt.Errorf("search query must be at least 3 characters long, got: %q", query)
    }

    // ❌ Bad: Generic error
    // return nil, fmt.Errorf("invalid input")
}
```

### 4. Context Awareness

Use context for cancellation and timeouts:

```go
func slowTool(ctx context.Context, args map[string]any) (any, error) {
    resultChan := make(chan result)

    go func() {
        // Long-running operation...
        resultChan <- fetchData()
    }()

    select {
    case <-ctx.Done():
        return nil, ctx.Err()  // Respect cancellation
    case r := <-resultChan:
        return r, nil
    }
}
```

### 5. Idempotency

Tools may be called multiple times with the same arguments:

```go
func incrementCounter(ctx context.Context, args map[string]any) (any, error) {
    // ❌ Bad: Side effects without idempotency checks
    counter++

    // ✅ Good: Idempotent design
    key := args["key"].(string)
    value, _ := cache.GetOrSet(key, 1)
    return value, nil
}
```

### 6. Input Validation

Always validate and sanitize inputs:

```go
func queryDB(ctx context.Context, args map[string]any) (any, error) {
    query := args["query"].(string)

    // ✅ Validate before using
    if strings.Contains(query, ";") {
        return nil, fmt.Errorf("invalid query: semicolons not allowed")
    }

    // ✅ Use parameterized queries
    rows, err := db.QueryContext(ctx, "SELECT * FROM users WHERE name = ?", query)
    // ...
}
```

## Advanced Usage

### Error Recovery

Tool errors are sent back to the model, allowing intelligent recovery:

```go
client.RegisterTool(
    "api_call",
    "Call external API",
    schema,
    func(ctx context.Context, args map[string]any) (any, error) {
        result, err := callExternalAPI(args)
        if err != nil {
            // Model receives this error and can:
            // 1. Retry with different parameters
            // 2. Try an alternative approach
            // 3. Inform the user
            return nil, fmt.Errorf("API call failed: %w", err)
        }
        return result, nil
    },
)
```

### Streaming with Tools

Tool calling works with streaming responses:

```go
stream, err := client.Text().
    Prompt("What's the weather?").
    Stream(ctx)

for chunk := range stream {
    if chunk.Error != nil {
        // Handle error
        continue
    }

    // Process tool calls as they arrive
    if len(chunk.ToolCalls) > 0 {
        // Tool calls in streaming mode
    }

    fmt.Print(chunk.Text)
}
```

### Manual Tool Execution

For fine-grained control, disable automatic execution:

```go
response, _ := client.Text().
    Prompt("What's the weather?").
    WithToolsDisabled().
    Generate(ctx)

// Manually execute tools
for _, toolCall := range response.ToolCalls {
    // Look up tool in registry
    definition := client.toolRegistry.Get(toolCall.Name)

    // Execute with custom logic
    result, err := definition.Handler(ctx, toolCall.Arguments)

    // Build result message manually
    // Send back to model for follow-up...
}
```

## Troubleshooting

### Tools Not Being Called

**Problem**: Model doesn't use registered tools

**Solutions**:
- ✅ Make tool descriptions clear and specific
- ✅ Verify schema matches expected inputs
- ✅ Check model supports function calling (gpt-3.5-turbo+, gpt-5+, claude-3+, claude-sonnet-4-5)
- ✅ Use explicit prompts: "Use the get_weather tool to..."

### Infinite Tool Loops

**Problem**: Tools called repeatedly without convergence

**Solutions**:
- ✅ Set `WithMaxToolIterations()` to reasonable limit (default: 10)
- ✅ Review tool outputs - ensure they provide useful data
- ✅ Check for circular dependencies between tools
- ✅ Add logging to see what's being called

### Type Assertion Errors

**Problem**: Runtime panic on type assertions

**Solutions**:
- ✅ Use safe type assertions: `if val, ok := args["key"].(string); ok`
- ✅ Define strict schemas to enforce types
- ✅ Validate all inputs before use
- ✅ Return errors instead of panicking

## See Also

- [Main README](../README.md) - Full Wormhole documentation
- [Examples](../examples/tool_calling/) - Complete working examples
- [API Reference](../API.md) - Complete API documentation
