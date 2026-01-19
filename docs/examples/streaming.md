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
    ctx := context.Background()

    client := wormhole.New(
        wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
        wormhole.WithDefaultProvider("openai"),
    )
    defer client.Close()

    // Start streaming
    stream, err := client.Text().
        Model("gpt-4o").
        Prompt("Write a haiku about Go programming").
        Stream(ctx)
    if err != nil {
        panic(err)
    }

    // Iterate over chunks as they arrive
    for chunk := range stream {
        // Check for errors in the stream
        if chunk.HasError() {
            fmt.Printf("\nError: %v\n", chunk.Error)
            return
        }

        // Print each chunk of text
        fmt.Print(chunk.Content())
    }

    fmt.Println("\n--- Stream complete ---")
}
```

## Error Handling

Always check for errors during streaming:

```go
stream, err := client.Text().
    Model("gpt-4o").
    Prompt("Tell me a story").
    Stream(ctx)
if err != nil {
    return err
}

for chunk := range stream {
    if chunk.HasError() {
        // Handle stream error
        return chunk.Error
    }

    fmt.Print(chunk.Content())

    // Check if this is the final chunk
    if chunk.IsDone() {
        break
    }
}
```

## Stream Accumulation

For applications that need both real-time output and the final result, use `StreamAndAccumulate()`:

```go
chunks, getResult, err := client.Text().
    Model("gpt-4o").
    Prompt("Explain goroutines in detail").
    StreamAndAccumulate(ctx)
if err != nil {
    return err
}

// Process chunks in real-time
for chunk := range chunks {
    if chunk.HasError() {
        return chunk.Error
    }
    fmt.Print(chunk.Content())
}

// Get the complete accumulated text
fullText := getResult()
fmt.Printf("\n\nTotal length: %d characters\n", len(fullText))
```

## Detecting Stream Completion

Use `IsDone()` to detect when streaming is complete:

```go
stream, _ := client.Text().
    Model("gpt-4o").
    Prompt("Count to 10").
    Stream(ctx)

for chunk := range stream {
    if chunk.HasError() {
        return chunk.Error
    }

    // Print content
    fmt.Print(chunk.Content())

    // Check if stream is complete
    if chunk.IsDone() {
        reason := *chunk.FinishReason
        fmt.Printf("\nStream finished: %s\n", reason)

        // Check usage if available
        if chunk.Usage != nil {
            fmt.Printf("Tokens: %d input, %d output, %d total\n",
                chunk.Usage.PromptTokens,
                chunk.Usage.CompletionTokens,
                chunk.Usage.TotalTokens,
            )
        }
    }
}
```

## Context Cancellation

Cancel streaming by canceling the context:

```go
ctx, cancel := context.WithCancel(context.Background())

stream, _ := client.Text().
    Model("gpt-4o").
    Prompt("Write a very long story").
    Stream(ctx)

chunkCount := 0
for chunk := range stream {
    if chunk.HasError() {
        return chunk.Error
    }

    fmt.Print(chunk.Content())
    chunkCount++

    // Cancel after 10 chunks
    if chunkCount >= 10 {
        cancel()
        break
    }
}

// Wait a moment for cleanup
time.Sleep(100 * time.Millisecond)
```

## Streaming with Different Providers

Streaming works uniformly across all providers:

```go
// Anthropic
stream, _ := client.Text().
    Using("anthropic").
    Model("claude-sonnet-4-5").
    Prompt("Hello").
    Stream(ctx)

// Gemini
stream, _ := client.Text().
    Using("gemini").
    Model("gemini-2.5-flash").
    Prompt("Hello").
    Stream(ctx)

// Ollama (local)
stream, _ := client.Text().
    Using("ollama").
    Model("llama3.2").
    Prompt("Hello").
    Stream(ctx)

// All providers use the same iteration pattern
for chunk := range stream {
    if chunk.HasError() {
        return chunk.Error
    }
    fmt.Print(chunk.Content())
}
```

## Server-Sent Events (HTTP)

For web applications, send chunks to clients via Server-Sent Events:

```go
func streamHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // Get prompt from query
    prompt := r.URL.Query().Get("prompt")

    // Start streaming
    stream, err := client.Text().
        Model("gpt-4o").
        Prompt(prompt).
        Stream(ctx)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Flush to ensure headers are sent
    flusher, _ := w.(http.Flusher)
    flusher.Flush()

    // Stream each chunk
    for chunk := range stream {
        if chunk.HasError() {
            fmt.Fprintf(w, "event: error\ndata: %s\n\n", chunk.Error)
            flusher.Flush()
            return
        }

        // Send SSE format
        fmt.Fprintf(w, "event: token\ndata: %s\n\n", chunk.Content())
        flusher.Flush()
    }

    // Send completion event
    fmt.Fprint(w, "event: done\ndata: {}\n\n")
    flusher.Flush()
}
```

## Streaming with Tool Calls

When the model uses tools, tool calls appear in the stream:

```go
client.RegisterTool(
    "get_time",
    "Get the current time",
    types.ObjectSchema{
        Type: "object",
        Properties: map[string]types.Schema{
            "timezone": types.StringSchema{Type: "string"},
        },
    },
    func(ctx context.Context, args map[string]any) (any, error) {
        return map[string]any{"time": time.Now().Format(time.RFC3339)}, nil
    },
)

stream, _ := client.Text().
    Model("gpt-4o").
    Prompt("What time is it?").
    Stream(ctx)

for chunk := range stream {
    if chunk.HasError() {
        return chunk.Error
    }

    // Check for tool calls
    if len(chunk.ToolCalls) > 0 {
        for _, tc := range chunk.ToolCalls {
            fmt.Printf("Tool call: %s with args: %v\n", tc.Name, tc.Arguments)
        }
    }

    // Print text content
    if chunk.Content() != "" {
        fmt.Print(chunk.Content())
    }
}
```

## Cleanup Best Practices

Always ensure streams are properly cleaned up:

```go
func streamWithCleanup(ctx context.Context) error {
    stream, err := client.Text().
        Model("gpt-4o").
        Prompt("Hello").
        Stream(ctx)
    if err != nil {
        return err
    }

    // Ensure we drain the channel even if we break early
    defer func() {
        for range stream {
            // Drain remaining chunks
        }
    }()

    for chunk := range stream {
        if chunk.HasError() {
            return chunk.Error
        }

        fmt.Print(chunk.Content())

        // Early exit condition
        if chunk.IsDone() {
            break
        }
    }

    return nil
}
```

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
