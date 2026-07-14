# DeepSeek Provider

DeepSeek is supported through Wormhole's OpenAI-compatible provider profile. There is no dedicated DeepSeek provider; the profile owns the default base URL, environment variables, and wire options for the OpenAI-compatible bridge.

Use `DEEPSEEK_API_KEY` for authentication. The default base URL is `https://api.deepseek.com`; set `DEEPSEEK_BASE_URL` to override it.

## Quick Start

### Creating a Client

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/garyblankenship/wormhole/v2/types"
    "github.com/garyblankenship/wormhole/v2"
)

func main() {
    client := wormhole.New(
        wormhole.WithDefaultProvider("deepseek"),
        wormhole.WithProfiledOpenAICompatible("deepseek", types.ProviderConfig{
            APIKey: os.Getenv("DEEPSEEK_API_KEY"),
        }),
    )

    ctx := context.Background()

    response, err := client.Text().
        Model("deepseek-v4-flash").
        Prompt("Explain Go interfaces in two sentences.").
        Generate(ctx)

    if err != nil {
        panic(err)
    }

    fmt.Println(response.Content())
}
```

## Models

Current V4 lineup:

| Model | Context | Max output | Notes |
|-------|---------|------------|-------|
| `deepseek-v4-pro` | 1M | 384K | Thinking (default on at DeepSeek) + non-thinking; tools, JSON |
| `deepseek-v4-flash` | 1M | 384K | Cheaper; thinking + non-thinking; tools, JSON |

Legacy IDs `deepseek-chat` and `deepseek-reasoner` are deprecated and DeepSeek retires them after 2026-07-24. They currently alias to `deepseek-v4-flash` non-thinking and thinking modes respectively. Prefer the V4 IDs.

## Reasoning (thinking)

Wormhole's DeepSeek profile sends `thinking: {"type": "disabled"}` by default for predictable, cheaper calls. To enable chain-of-thought, pass DeepSeek options through `ProviderOptions`:

```go
response, err := client.Text().
    Model("deepseek-v4-pro").
    Prompt("Use json to explain the tradeoffs.").
    ProviderOptions(map[string]any{
        "thinking": map[string]any{"type": "enabled"},
        "reasoning_effort": "high",
    }).
    Generate(ctx)
```

When thinking is enabled, DeepSeek returns chain-of-thought in `reasoning_content`. Wormhole surfaces it as `response.Thinking` (`*types.Thinking`, with `.Content`) for non-streaming responses and as `chunk.Thinking` while streaming. `reasoning_effort` accepts `high` (default) or `max`.

## Caching

DeepSeek performs automatic context caching. Cache hits are reported as `response.Usage.CacheReadTokens`, mapped from DeepSeek's `prompt_cache_hit_tokens`.

## Tool calling / JSON

Tool calling is supported through OpenAI-compatible `tools` and `tool_choice`. JSON mode is supported with `response_format: {"type":"json_object"}`; include the word "json" in the prompt, which DeepSeek requires.

## Not supported here

Beta-only features such as chat prefix completion, FIM, and strict tool mode require DeepSeek's `/beta` base URL and are outside Wormhole's bridge scope. Use the DeepSeek API directly for those: https://api-docs.deepseek.com/
