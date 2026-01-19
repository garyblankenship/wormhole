# Streaming Responses

Streaming responses enable real-time text output as the model generates each token. This is essential for chat interfaces, interactive applications, and improving perceived latency.

## Basic Streaming

The simplest way to stream is to iterate over the channel returned by `Stream()`:

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
    ctx := context.Background()                     // [1] Create context for cancellation/timeout control

    client := wormhole.New(                         // [2] Initialize client with provider config
        wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),  // [2a] Pass OpenAI API key from env
        wormhole.WithDefaultProvider("openai"),     // [2b] Set default provider (can switch later)
    )
    defer client.Close()                            // [3] Clean up resources when function exits

    // Start streaming
    stream, err := client.Text().                   // [4] Create text request builder
        Model("gpt-4o").                            // [4a] Specify model to use
        Prompt("Write a haiku about Go programming"). // [4b] Set the user prompt
        Stream(ctx)                                 // [4c] Execute request, returns read-only channel
    if err != nil {
        panic(err)
    }

    // Iterate over chunks as they arrive
    for chunk := range stream {                     // [5] Range over channel - blocks until chunks arrive
        // Check for errors in the stream
        if chunk.HasError() {                       // [6] Errors arrive as final chunk in stream
            fmt.Printf("\nError: %v\n", chunk.Error)
            return
        }

        // Print each chunk of text
        fmt.Print(chunk.Content())                  // [7] Content() returns unified text (handles Text/Delta fields)
    }

    fmt.Println("\n--- Stream complete ---")
}
```

### What's happening?

1. **Context creation** - A background context is created. This can be used for timeout control or cancellation if needed.
2. **Client initialization** - The client is configured with an OpenAI API key and the default provider is set. The functional options pattern allows flexible configuration.
3. **Resource cleanup** - `defer client.Close()` ensures resources are released when the function exits, even if a panic occurs.
4. **Request building** - The fluent builder pattern chains configuration. `Stream()` starts the request and returns a channel.
5. **Channel iteration** - The `for chunk := range stream` syntax reads from the channel as chunks arrive from the provider.
6. **Error checking** - Each chunk may contain an error. `HasError()` is a helper to detect this without type assertion.
7. **Content output** - `Content()` is a unified accessor that returns text regardless of which field the provider uses (`Text` or `Delta.Content`).

> [!TIP]
> Streaming provides better perceived latency even when total time is similar to non-streaming. Users see tokens appear as they generate rather than waiting for the complete response.

> [!WARNING]
> Always iterate over the entire stream channel to avoid goroutine leaks. Breaking early without draining the channel leaves the streaming goroutine running.

## Error Handling

Always check for errors during streaming:

```go
stream, err := client.Text().                    // [1] Build streaming request
    Model("gpt-4o").
    Prompt("Tell me a story").
    Stream(ctx)                                  // [1a] Returns error immediately if setup fails
if err != nil {
    return err                                   // [2] Initial setup errors (auth, config) return here
}

for chunk := range stream {                      // [3] Iterate over streaming chunks
    if chunk.HasError() {                        // [4] Stream errors arrive inside the channel
        // Handle stream error
        return chunk.Error                       // [5] Stream errors (timeout, network) return here
    }

    fmt.Print(chunk.Content())                   // [6] Print content as it arrives

    // Check if this is the final chunk
    if chunk.IsDone() {                          // [7] Detect completion without breaking
        break                                    // [8] Early exit - stream finished successfully
    }
}
```

### What's happening?

1. **Stream initiation** - Errors during request setup (invalid API key, bad model name) return immediately, not through the channel.
2. **Early return on setup failure** - If `Stream()` returns an error, we immediately return it without entering the loop.
3. **Channel iteration** - Successful requests return a channel. We iterate as chunks arrive.
4. **Error detection in-stream** - Network failures, timeouts, and provider errors arrive as chunks with `HasError() == true`.
5. **Stream error return** - When an error chunk arrives, we return it. This may be retryable depending on error type.
6. **Content output** - Print content as it arrives for real-time feedback.
7. **Completion detection** - `IsDone()` checks if this is the final chunk (has `FinishReason` set).
8. **Early exit** - Breaking early is safe here since we detected proper completion.

> [!WARNING]
> There are two error paths in streaming: (1) immediate errors from `Stream()` call for setup failures, and (2) errors delivered through the channel for runtime failures. Handle both.

## Stream Accumulation

For applications that need both real-time output and the final result, use `StreamAndAccumulate()`:

```go
chunks, getResult, err := client.Text().         // [1] Build streaming request
    Model("gpt-4o").
    Prompt("Explain goroutines in detail").
    StreamAndAccumulate(ctx)                     // [2] Returns: channel, accumulator func, error
if err != nil {
    return err
}

// Process chunks in real-time
for chunk := range chunks {                      // [3] Iterate over stream channel
    if chunk.HasError() {
        return chunk.Error
    }
    fmt.Print(chunk.Content())                   // [4] Display to user in real-time
}

// Get the complete accumulated text
fullText := getResult()                          // [5] Call accumulator function after stream ends
fmt.Printf("\n\nTotal length: %d characters\n", len(fullText))
```

### What's happening?

1. **Request building** - Create the text generation request with model and prompt.
2. **Stream and accumulate** - `StreamAndAccumulate()` returns three values: the chunk channel, a function to retrieve the accumulated text, and an error.
3. **Channel iteration** - Process chunks as they arrive from the provider.
4. **Real-time display** - Print each chunk immediately for the streaming effect.
5. **Accumulator retrieval** - After the channel closes, call `getResult()` to get the complete accumulated text as a single string.

> [!TIP]
> Use `StreamAndAccumulate()` when you need both the streaming user experience AND the complete text for post-processing (e.g., saving to database, further analysis).

> [!WARNING]
> Call `getResult()` only after consuming the entire channel. The accumulator runs concurrently and updates as chunks arrive.

## Detecting Stream Completion

Use `IsDone()` to detect when streaming is complete:

```go
stream, _ := client.Text().                       // [1] Create streaming request
    Model("gpt-4o").
    Prompt("Count to 10").
    Stream(ctx)

for chunk := range stream {                       // [2] Iterate over chunks
    if chunk.HasError() {
        return chunk.Error
    }

    // Print content
    fmt.Print(chunk.Content())                    // [3] Output text as it arrives

    // Check if stream is complete
    if chunk.IsDone() {                           // [4] Detect final chunk
        reason := *chunk.FinishReason             // [5] Get why stream ended
        fmt.Printf("\nStream finished: %s\n", reason)

        // Check usage if available
        if chunk.Usage != nil {                   // [6] Final chunk contains usage stats
            fmt.Printf("Tokens: %d input, %d output, %d total\n",
                chunk.Usage.PromptTokens,         // [7] Input tokens sent to model
                chunk.Usage.CompletionTokens,     // [8] Tokens generated by model
                chunk.Usage.TotalTokens,          // [9] Sum of input + output
            )
        }
    }
}
```

### What's happening?

1. **Stream creation** - Start a streaming request.
2. **Channel iteration** - Loop through chunks as they arrive.
3. **Content output** - Print each chunk for real-time display.
4. **Completion detection** - `IsDone()` returns `true` for the final chunk, which contains metadata.
5. **Finish reason** - Values include `stop` (complete), `length` (max tokens), `content_filter` (flagged), etc.
6. **Usage availability** - Only the final chunk contains token usage information.
7. **Prompt tokens** - Tokens from your input (messages + system prompt).
8. **Completion tokens** - Tokens the model generated.
9. **Total tokens** - Used for cost calculation.

> [!TIP]
> The `FinishReason` tells you why the stream ended: `stop` means the model naturally completed, `length` means it hit `max_tokens`, and `content_filter` means the content was flagged.

## Context Cancellation

Cancel streaming by canceling the context:

```go
ctx, cancel := context.WithCancel(context.Background())  // [1] Create cancellable context

stream, _ := client.Text().                               // [2] Start streaming with cancellable context
    Model("gpt-4o").
    Prompt("Write a very long story").
    Stream(ctx)

chunkCount := 0
for chunk := range stream {                               // [3] Iterate over stream
    if chunk.HasError() {
        return chunk.Error
    }

    fmt.Print(chunk.Content())
    chunkCount++

    // Cancel after 10 chunks
    if chunkCount >= 10 {                                 // [4] Check cancellation condition
        cancel()                                          // [5] Signal cancellation to stream
        break                                             // [6] Exit loop
    }
}

// Wait a moment for cleanup
time.Sleep(100 * time.Millisecond)                       // [7] Allow goroutine cleanup
```

### What's happening?

1. **Cancellable context** - `WithCancel` creates a context that can be cancelled by calling the returned `cancel()` function.
2. **Stream with context** - The stream goroutine monitors the context for cancellation signals.
3. **Channel iteration** - Process chunks as they arrive.
4. **Cancellation condition** - Check your business logic for when to stop streaming (token count, keyword, time limit, etc.).
5. **Trigger cancellation** - Calling `cancel()` sends a signal to any goroutines watching this context.
6. **Exit loop** - After triggering cancellation, break from the loop.
7. **Cleanup grace period** - Give the streaming goroutine time to receive the cancellation signal and clean up resources.

> [!WARNING]
> Always call `cancel()` to release context resources. Failing to cancel contexts can lead to goroutine and memory leaks, especially in long-running applications.

> [!TIP]
> For time-based cancellation, use `context.WithTimeout()` instead of manual counting:
> ```go
> ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
> defer cancel()
> ```

## Streaming with Different Providers

Streaming works uniformly across all providers:

```go
// Anthropic
stream, _ := client.Text().                       // [1] Anthropic streaming
    Using("anthropic").                           // [1a] Switch to Anthropic provider
    Model("claude-sonnet-4-5").
    Prompt("Hello").
    Stream(ctx)

// Gemini
stream, _ := client.Text().                       // [2] Gemini streaming
    Using("gemini").                              // [2a] Switch to Gemini provider
    Model("gemini-2.5-flash").
    Prompt("Hello").
    Stream(ctx)

// Ollama (local)
stream, _ := client.Text().                       // [3] Ollama streaming
    Using("ollama").                              // [3a] Switch to Ollama (local) provider
    Model("llama3.2").
    Prompt("Hello").
    Stream(ctx)

// All providers use the same iteration pattern
for chunk := range stream {                       // [4] Universal iteration pattern
    if chunk.HasError() {
        return chunk.Error
    }
    fmt.Print(chunk.Content())
}
```

### What's happening?

1. **Anthropic streaming** - Switch providers using `Using()` method. The same fluent builder pattern works.
2. **Gemini streaming** - No code changes needed compared to Anthropic - just provider name and model.
3. **Ollama streaming** - Local models work identically to cloud providers.
4. **Universal iteration** - The same loop works for all providers - Wormhole normalizes differences internally.

> [!TIP]
> Provider switching is instant (~67ns overhead). You can even switch providers mid-application without creating a new client.

> [!WARNING]
> Ensure you've configured API keys for all providers you use. Ollama and other local providers don't require keys, but cloud providers do.

## Server-Sent Events (HTTP)

For web applications, send chunks to clients via Server-Sent Events:

```go
func streamHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()                               // [1] Use request context for cancellation

    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")  // [2] MIME type for SSE
    w.Header().Set("Cache-Control", "no-cache")      // [3] Prevent buffering of responses
    w.Header().Set("Connection", "keep-alive")       // [4] Maintain persistent connection

    // Get prompt from query
    prompt := r.URL.Query().Get("prompt")            // [5] Extract user input

    // Start streaming
    stream, err := client.Text().                    // [6] Create streaming request
        Model("gpt-4o").
        Prompt(prompt).
        Stream(ctx)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Flush to ensure headers are sent
    flusher, _ := w.(http.Flusher)                   // [7] Type assert to Flusher interface
    flusher.Flush()                                  // [8] Send headers immediately

    // Stream each chunk
    for chunk := range stream {                       // [9] Iterate over stream chunks
        if chunk.HasError() {
            fmt.Fprintf(w, "event: error\ndata: %s\n\n", chunk.Error)  // [10] Error event
            flusher.Flush()
            return
        }

        // Send SSE format
        fmt.Fprintf(w, "event: token\ndata: %s\n\n", chunk.Content())  // [11] Token event
        flusher.Flush()                              // [12] Push data to client immediately
    }

    // Send completion event
    fmt.Fprint(w, "event: done\ndata: {}\n\n")       // [13] Signal completion
    flusher.Flush()
}
```

### What's happening?

1. **Request context** - Using the request's context allows automatic cancellation if the client disconnects.
2. **SSE content type** - Tells the browser this is an SSE endpoint, not a regular HTTP response.
3. **No cache** - Prevents proxies and browsers from caching the streaming response.
4. **Keep-alive** - Maintains the HTTP connection open for multiple events.
5. **Query parameter** - Extract the user's prompt from the URL query string.
6. **Start streaming** - Begin the AI generation request.
7. **Flusher interface** - ResponseWriter must implement `http.Flusher` for SSE to work.
8. **Initial flush** - Send HTTP headers immediately so the client knows to start listening for events.
9. **Stream loop** - Process chunks from the AI provider.
10. **Error event** - SSE event format: `event: error` tells client this is an error.
11. **Token event** - SSE event format: `event: token` with the chunk content as data.
12. **Per-chunk flush** - Critical - without flushing, data buffers and streaming effect is lost.
13. **Done event** - Send final event so client knows the stream is complete.

> [!TIP]
> SSE format: `event: <type>\ndata: <payload>\n\n`. The double newline ends each event. Multiple data lines can be sent before the closing `\n\n`.

> [!WARNING]
> Many proxies buffer SSE responses. Use `no-cache` headers and consider running over HTTPS to prevent proxy interference. Some load balancers may drop idle connections.

## Streaming with Tool Calls

When the model uses tools, tool calls appear in the stream:

```go
client.RegisterTool(                                   // [1] Register tool for AI to call
    "get_time",
    "Get the current time",
    types.ObjectSchema{
        Type: "object",
        Properties: map[string]types.Schema{
            "timezone": types.StringSchema{Type: "string"},
        },
    },
    func(ctx context.Context, args map[string]any) (any, error) {  // [2] Tool implementation
        return map[string]any{"time": time.Now().Format(time.RFC3339)}, nil
    },
)

stream, _ := client.Text().                            // [3] Start streaming with tool available
    Model("gpt-4o").
    Prompt("What time is it?").
    Stream(ctx)

for chunk := range stream {                            // [4] Iterate over stream chunks
    if chunk.HasError() {
        return chunk.Error
    }

    // Check for tool calls
    if len(chunk.ToolCalls) > 0 {                      // [5] Tool calls arrive in chunks
        for _, tc := range chunk.ToolCalls {
            fmt.Printf("Tool call: %s with args: %v\n", tc.Name, tc.Arguments)
        }
    }

    // Print text content
    if chunk.Content() != "" {                         // [6] Regular text also appears
        fmt.Print(chunk.Content())
    }
}
```

### What's happening?

1. **Tool registration** - Register a Go function that the AI can call. The schema tells the AI what parameters the tool accepts.
2. **Tool handler** - This function runs when the AI decides to use the tool. It returns the result as a map.
3. **Start streaming** - Begin the generation. The AI may decide to call tools during generation.
4. **Stream iteration** - Process chunks as they arrive.
5. **Tool call detection** - When the AI wants to use a tool, `ToolCalls` will be populated with the tool name and arguments.
6. **Content output** - Regular text output continues alongside tool calls.

> [!TIP]
> Tool calls in streaming arrive incrementally. Arguments may be split across multiple chunks. The complete tool call is available when `IsDone()` is true for that chunk.

> [!WARNING]
> The SDK handles multi-turn conversations automatically when tools are used. After a tool executes, the result is sent back to the AI, which generates a final response. This is transparent to you.

## Cleanup Best Practices

Always ensure streams are properly cleaned up:

```go
func streamWithCleanup(ctx context.Context) error {
    stream, err := client.Text().                    // [1] Create stream
        Model("gpt-4o").
        Prompt("Hello").
        Stream(ctx)
    if err != nil {
        return err
    }

    // Ensure we drain the channel even if we break early
    defer func() {                                    // [2] Deferred cleanup runs on any exit path
        for range stream {                            // [3] Drain channel to prevent goroutine leak
            // Drain remaining chunks
        }
    }()

    for chunk := range stream {                       // [4] Main processing loop
        if chunk.HasError() {
            return chunk.Error
        }

        fmt.Print(chunk.Content())

        // Early exit condition
        if chunk.IsDone() {                           // [5] Check for completion
            break                                     // [6] Safe to break - defer will drain
        }
    }

    return nil
}
```

### What's happening?

1. **Stream creation** - Start the streaming request.
2. **Deferred cleanup** - The `defer` ensures cleanup runs regardless of how the function exits (return, panic, break).
3. **Channel draining** - Draining the channel allows the streaming goroutine to complete and be garbage collected.
4. **Main loop** - Process chunks normally.
5. **Completion check** - Use `IsDone()` to detect proper stream completion.
6. **Early break** - Breaking early is safe because the defer will drain the remaining channel.

> [!TIP]
> The defer pattern is the safest way to handle streaming cleanup. It ensures the channel is always drained, preventing goroutine leaks even if you return early or a panic occurs.

> [!WARNING]
> Never break from a stream loop without draining the channel (either in a defer or after the loop). Abandoned channels cause goroutine leaks that accumulate over time.

## Chunk Structure

Each `StreamChunk` provides access to:

| Field | Type | Description |
|-------|------|-------------|
| `Content()` | `string` | Text content (unified accessor) |
| `Text` | `string` | Raw text field |
| `Delta.Content` | `string` | OpenAI-style delta content |
| `ToolCalls` | `[]ToolCall` | Tool calls in this chunk |
| `FinishReason` | `*FinishReason` | Why stream ended |
| `Usage` | `*Usage` | Token usage (final chunk) |
| `Error` | `error` | Error if any |
| `IsDone()` | `bool` | Check if final chunk |
| `HasError()` | `bool` | Check if error present |
