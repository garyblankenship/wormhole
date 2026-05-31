# OpenAI Provider

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
    client := wormhole.New(
        wormhole.WithDefaultProvider("openai"),
        wormhole.WithOpenAIResponses(os.Getenv("OPENAI_API_KEY")),
    )
    defer client.Close()

    resp, err := client.Text().Model("gpt-5").Prompt("Say hello.").Generate(context.Background())
    if err != nil { panic(err) }
    fmt.Println(resp.Content())
}
```

The OpenAI provider supports text, streaming, structured output, embeddings,
images, audio, and tool calling. Text generation uses Chat Completions by
default, and you can opt into the OpenAI Responses API for `/v1/responses`.

## Basic Chat Completions

Use `WithOpenAI` for the default Chat Completions wire format:

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
    client := wormhole.New(
        wormhole.WithDefaultProvider("openai"),
        wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    )
    defer client.Close()

    resp, err := client.Text().
        Model("gpt-5").
        Prompt("Write one sentence about Go interfaces.").
        Generate(context.Background())
    if err != nil {
        panic(err)
    }

    fmt.Println(resp.Content())
}
```

## Responses API

Use `WithOpenAIResponses` when you want OpenAI text requests to use
`POST /v1/responses`:

```go
client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAIResponses(os.Getenv("OPENAI_API_KEY")),
)
```

The same `Text()` builder still drives the request:

```go
resp, err := client.Text().
    Model("gpt-5").
    SystemPrompt("Answer tersely.").
    Prompt("What is a type parameter?").
    MaxTokens(128).
    Generate(ctx)
```

Responses mode maps common text fields to the Responses wire format:

| Wormhole field | Responses API field |
| --- | --- |
| `Model()` | `model` |
| `Prompt()`, `Messages()`, `SystemPrompt()` | `input` items |
| `Temperature()` | `temperature` |
| `TopP()` | `top_p` |
| `MaxTokens()` | `max_output_tokens` |
| `Tools()` | `tools` |
| `ToolChoice()` | `tool_choice` |
| `ResponseFormat()` | `text.format` |
| `ProviderOptions()` | merged into the request body |

`WithOpenAIResponses` is intentionally OpenAI-specific. Generic
OpenAI-compatible providers keep using Chat Completions unless you configure
their `ProviderConfig` manually, because many local or gateway providers do not
implement `/responses`.

## Provider Config

| Field | Type | Default | Source |
| --- | --- | --- | --- |
| `BaseURL` | `string` | `https://api.openai.com/v1` | `pkg/providers/openai/openai.go` |
| `ChatPath` | `string` | `/chat/completions` | `pkg/providers/openai/openai.go` |
| `UseResponsesAPI` | `bool` | `false` | `pkg/types/provider.go` |
| `ResponsesPath` | `string` | `/responses` | `pkg/providers/openai/openai.go` |

Configure the Responses path only when you are targeting a compatible proxy:

```go
cfg := types.ProviderConfig{
    BaseURL:         "http://localhost:8080",
    UseResponsesAPI: true,
    ResponsesPath:   "/v1/responses",
}

client := wormhole.New(
    wormhole.WithDefaultProvider("openai"),
    wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY"), cfg),
)
```

## Structured Output

`ResponseFormat()` is sent as `text.format` in Responses mode:

```go
resp, err := client.Text().
    Model("gpt-5").
    Prompt("Return JSON for Ada Lovelace with name and role.").
    ResponseFormat(map[string]any{"type": "json_object"}).
    Generate(ctx)
```

For typed extraction, keep using `client.Structured()`. It is routed through the
configured OpenAI provider and receives the same normalized `StructuredResponse`
shape as other providers.

## Tool Calling

Responses mode translates Wormhole tools into the Responses function-tool shape
and normalizes returned `function_call` items into `types.ToolCall`:

```go
tool := types.NewTool("lookup", "Lookup a record", map[string]any{
    "type": "object",
    "properties": map[string]any{
        "q": map[string]any{"type": "string"},
    },
    "required": []string{"q"},
})

resp, err := client.Text().
    Model("gpt-5").
    Prompt("Look up Ada Lovelace.").
    Tools(*tool).
    Generate(ctx)

if len(resp.ToolCalls) > 0 {
    fmt.Println(resp.ToolCalls[0].Name)
}
```

## Streaming

Streaming works through the same builder. In Responses mode, OpenAI
`response.output_text.delta` events become `types.TextChunk` values.

```go
stream, err := client.Text().
    Model("gpt-5").
    Prompt("Count to three.").
    Stream(ctx)
if err != nil {
    panic(err)
}

for chunk := range stream {
    if chunk.HasError() {
        panic(chunk.Error)
    }
    fmt.Print(chunk.Content())
}
```

The final chunk carries provider, response ID, finish reason, and usage when the
upstream response includes them.
