# Messages

Messages represent the communication units between users and AI models in
the Wormhole SDK.

## Message Interface

All message types implement the `Message` interface:

```go
type Message interface {
    GetRole() Role
    GetContent() any
}
```

## Role Types

Each message has a `Role` that identifies who sent it:

| Role     | Constant        | Description                               |
| -------- | --------------- | ----------------------------------------- |
| system   | RoleSystem      | Sets behavior/context for the assistant   |
| user     | RoleUser        | Human user input                          |
| assistant| RoleAssistant   | AI model response                         |
| tool     | RoleTool        | Function execution result                 |

## Message Types

### SystemMessage

Sets context, instructions, or behavior constraints for the assistant.

```go
msg := types.NewSystemMessage("You are a helpful assistant specializing in Go programming.")
```

**Fields:**

- `Content string` - The system prompt text

### UserMessage

Represents input from the human user. Can include text and optional media.

```go
msg := types.NewUserMessage("What is the best way to handle errors in Go?")
```

**Fields:**

- `Content string` - The user's text message
- `Media []Media` - Optional attachments (images, documents)

**With media:**

```go
msg := &types.UserMessage{
    Content: "Describe this image",
    Media: []types.Media{
        &types.ImageMedia{
            Data:       imageBytes,
            MimeType:  "image/png",
        },
    },
}
```

### AssistantMessage

Represents the AI model's response. May include tool calls for
function execution.

```go
msg := types.NewAssistantMessage(
    "In Go, errors are values that should be handled explicitly.",
)
```

**Fields:**

- `Content string` - The assistant's text response
- `ToolCalls []ToolCall` - Optional function calls the assistant wants to make

### ToolResultMessage

Contains the result of executing a function/tool call.

```go
msg := types.NewToolResultMessage(toolCallID, "Execution successful")
```

**Fields:**

- `Content string` - Automatically generated successes contain raw JSON, including quoted strings and `null`; failures contain readable error text.
- `Error string` - Failure information for generated tool results, including serialization failures.
- `ToolCallID string` - Links to the original tool call

Wormhole prepares SDK system prompts into the message history once. The
provider dispatch copy clears `SystemPrompt` after that preparation; caller
inputs remain unchanged. Direct provider calls retain their own prompt handling.

Tool history is matched in order: each result consumes one unmatched earlier
call. Reusing an ID later is valid, but duplicate normalized IDs within one
assistant group fail validation. History repair preserves retained assistant
text, thinking, and signatures.

## Multi-Turn Conversations

A conversation is a sequence of messages representing the complete interaction history.

### Simple Exchange

```go
messages := []types.Message{
    types.NewSystemMessage("You are a Go expert."),
    types.NewUserMessage("What is a goroutine?"),
}
```

### Multi-Turn with History

```go
messages := []types.Message{
    types.NewSystemMessage("You are a helpful programming tutor."),

    // Turn 1
    types.NewUserMessage("Explain defer statements in Go."),
    types.NewAssistantMessage("A defer statement pushes a function call onto a list..."),

    // Turn 2
    types.NewUserMessage("Can you show me an example?"),
    types.NewAssistantMessage("Sure! Here's a practical example..."),

    // Turn 3 (current)
    types.NewUserMessage("What happens with multiple defers?"),
}
```

### With Tool Calls

```go
messages := []types.Message{
    types.NewSystemMessage("You have access to a weather API."),

    types.NewUserMessage("What's the weather in Boston?"),
    &types.AssistantMessage{
        ToolCalls: []types.ToolCall{
            {ID: "call_1", Name: "get_weather", Arguments: `{"city": "Boston"}`},
        },
    },
    types.NewToolResultMessage("call_1", `{"temp": 45, "condition": "sunny"}`),
    types.NewAssistantMessage("The weather in Boston is 45°F and sunny."),
}
```

## Best Practices

1. **Order matters**: Messages must be in chronological order
2. **System first**: Place system messages at the beginning of the
   conversation
3. **Alternating roles**: User and assistant messages should alternate
   (after any system message)
4. **Tool call flow**: Assistant with tool calls → tool results → assistant
   with final response
5. **Context management**: Include conversation history for stateful
   interactions
