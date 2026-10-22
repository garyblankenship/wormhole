# OpenAI Provider

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/garyblankenship/wormhole/v3"
)

func main() {
    client := wormhole.New(
        wormhole.WithDefaultProvider("openai"),
        wormhole.WithOpenAIResponses(os.Getenv("OPENAI_API_KEY")),
    )
    defer client.Close()

    resp, err := client.Text().Model("gpt-5.6").Prompt("Say hello.").Generate(context.Background())
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

    "github.com/garyblankenship/wormhole/v3"
)

func main() {
    client := wormhole.New(
        wormhole.WithDefaultProvider("openai"),
        wormhole.WithOpenAI(os.Getenv("OPENAI_API_KEY")),
    )
    defer client.Close()

    resp, err := client.Text().
        Model("gpt-5.6").
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
    Model("gpt-5.6").
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
| `BaseURL` | `string` | `https://api.openai.com/v1` | `providers/openai/openai.go` |
| `ChatPath` | `string` | `/chat/completions` | `providers/openai/openai.go` |
| `UseResponsesAPI` | `bool` | `false` | `types/provider.go` |
| `ResponsesPath` | `string` | `/responses` | `providers/openai/openai.go` |

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
    Model("gpt-5.6").
    Prompt("Return JSON for Ada Lovelace with name and role.").
    ResponseFormat(map[string]any{"type": "json_object"}).
    Generate(ctx)
```

For typed extraction, keep using `client.Structured()`. It is routed through the
configured OpenAI provider and receives the same normalized `StructuredResponse`
shape as other providers.

Strict structured output clones the schema before dispatch. Each object gains
`additionalProperties: false` when absent. Explicitly permissive
`additionalProperties` and properties omitted from `required` are rejected with
a schema field path before HTTP I/O. Represent an optional value as a required
nullable field, for example `"type": ["string", "null"]`; the property must still
appear in `required`. Nonstrict schemas remain unchanged.

## oMLX Thinking Controls

Use the existing `ProviderOptions()` escape hatch for oMLX's model-specific
chat-template fields. Supply `baseURL`, `modelID`, and `prompt` from your
application configuration/input; `baseURL` is the OpenAI-compatible API root
(normally ending in `/v1`).

```go
client := wormhole.New(
    wormhole.WithLocalOpenAI(baseURL),
    wormhole.WithDiscovery(false),
)
defer client.Close()

resp, err := client.Text().
    Model(modelID).
    Prompt(prompt).
    ResponseFormat(map[string]any{"type": "json_object"}).
    ProviderOptions(map[string]any{
        "chat_template_kwargs": map[string]any{
            "enable_thinking": false,
        },
    }).
    Generate(ctx)
```

This sends `"chat_template_kwargs":{"enable_thinking":false}` at the top level
of the chat-completions request body. The same `ProviderOptions()` field works
with `Stream()` and `Structured()` requests. Explicit `false` remains a JSON
boolean; omitting the field leaves the server's behavior unchanged when no
provider or model defaults supply it.

Provider options follow **defaults < model < request** precedence. Configure
those layers with `ProviderConfig.WithDefaultProviderOptions()`,
`ProviderConfig.WithProviderOptionsForModel()`, and the request's
`ProviderOptions()`. Each top-level key is replaced, not recursively merged:
a request's `chat_template_kwargs` replaces that entire model/default value,
so include every template kwarg you want to retain.

Important limits:

- The loaded model's chat template must support `enable_thinking`. Wormhole
  forwards the field; it cannot guarantee the server or model honors it.
  [Upstream oMLX discussion #1108](https://github.com/jundot/omlx/discussions/1108)
  reports this control working for Qwen; that is not LFM 24B acceptance evidence.
- `Reasoning(types.Reasoning{Effort: types.ReasoningEffortNone})` is a separate,
  provider-dependent effort hint, not a replacement for this template flag.
- `response_format` requests JSON output independently of thinking controls.
  Valid JSON does not establish application-schema compliance; validate the
  returned object against your contract, or use strict structured output when
  the server supports it.
- `WithLocalOpenAI` names the SDK provider `local`; it does not create a model
  alias. `Model(modelID)` sends the supplied ID or server-configured alias.
  A reported model name alone does not attest which underlying weights ran.

## Proxy Chat-Template Pass-Through

The standalone OpenAI-compatible proxy (`cmd/wormhole`) accepts the
`chat_template_kwargs` extension at the top level of `/v1/chat/completions`
bodies and forwards it to OpenAI-chat-compatible upstreams — the same field the
`ProviderOptions()` escape hatch sends for direct SDK use. Normal and streaming
chat handle the field identically.

```json
{
  "model": "vllm/qwen3-32b",
  "messages": [{"role": "user", "content": "hi"}],
  "chat_template_kwargs": {"enable_thinking": false}
}
```

The proxy validates the extension before contacting any upstream:

- `chat_template_kwargs` must be a JSON object; an explicit `null`, scalar, or
  array value is rejected with `400 invalid_request_error`.
- `enable_thinking`, when present, must be a JSON boolean; strings, numbers,
  objects, and null are rejected the same way.
- Native `anthropic/`, `gemini/`, and `ollama/` routes (and an equivalent
  default provider) are rejected with `unsupported_parameter`: those APIs never
  read the field, so a passthrough would silently drop the caller's intent.
  OpenAI and all OpenAI-chat-compatible routes accept it.

Other template kwargs pass through unchanged inside the object. Only this one
nested field is merged into the upstream body through provider options, so
routing, model, messages, authentication, and limits cannot be overridden
through it.

The same model/template boundary as direct SDK use applies: the upstream server
decides whether a template kwarg is meaningful for the loaded model's chat
template, and the proxy cannot make a model honor `enable_thinking`. A request's
`chat_template_kwargs` replaces any provider- or model-configured value as a
whole (top-level key replacement, not a recursive merge), so include every
template kwarg you want to retain.

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
    Model("gpt-5.6").
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
    Model("gpt-5.6").
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

OpenAI tool streams consume fragments internally; complete calls arrive
once in the terminal chunk's `ToolCalls`, each stamped with its wire index.
Reused or trailing events do not re-emit them. See
[tool streaming](../tool-calling.md#streaming-with-tools).
