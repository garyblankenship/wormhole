# Provider fidelity and application controls

Wormhole bridges the application-facing provider APIs. These controls preserve
provider details without adding file management, search orchestration, automatic
tool execution, or provider administration. Use official provider SDKs or REST
APIs to upload files, manage resources, or run administrative jobs.

## Response metadata

HTTP-backed responses expose `ResponseMetadata`: the registered provider,
upstream request ID, status code, and a filtered `Headers map[string][]string`.
`WormholeError.ResponseMetadata` preserves those details for provider failures
and response-decoding failures. Metadata describes the exposed response, not
every hidden HTTP transport retry. Logical attempt tracing has the same limit.

The default filter includes provider request-ID headers, `retry-after`, and
supported OpenAI-style and Anthropic rate-limit headers. Add SDK-visible names
with `ProviderConfig.ResponseHeaderNames`; matching ignores case. Authorization
credentials, cookies, and hop-by-hop headers are always excluded. Metadata is
per response, so concurrent requests do not share a mutable last-response slot.

Custom adapters can use `DoRequestWithMetadata`, `StreamRequestWithMetadata`,
and `DoRawRequestWithMetadata` on the HTTP wrapper. Existing wrapper methods
remain available and delegate to these variants.

Streaming attaches metadata to the first emitted chunk and the terminal chunk.
It does not emit a new metadata-only first chunk. Errors after connection retain
the available metadata. Keep consuming until completion or cancel the request
context when abandoning a stream.

The proxy forwards only its fixed safe request-ID and rate-limit allowlist and
sets `X-Wormhole-Upstream-Request-ID`. SDK-configured extra headers stay private
to SDK responses. Once streaming response headers are sent, later failures use
the existing SSE error path.

## Proxy opt-in extensions

Use the existing `/v1/chat/completions` endpoint:

```json
{
  "model": "openai/gpt-5.6",
  "messages": [{"role": "user", "content": "Explain this result."}],
  "wormhole": {
    "include": ["response_metadata", "usage_details", "sources", "citations", "warnings"]
  }
}
```

Each extension field is optional. To use the synthetic result tool described
below, set `structured_mode` to `"tools"` and supply a `response_format` of type
`"json_schema"` with a schema; do not combine it with `strict: true`.
Requested extra data appears under a
response-level `wormhole` object. Streams place the complete extension snapshot
on the terminal completion chunk before `[DONE]`. Standard tool calls, usage,
and representable native annotations retain their ordinary locations.

Unknown extension fields and unknown `include` values return HTTP 400. The
extension does not expose arbitrary provider options. Requests without it retain
the default proxy response shape.

## Cache controls and accounting

Keep native retention and cache controls in `ProviderOptions`. Provider
defaults are overridden by model options, then request options. There is no
universal cache-duration setting: native options and eligibility rules differ.

Normalized `Usage` reports available cache-read and cache-write tokens from
OpenAI Responses, Anthropic, and Gemini. `CacheWriteTokensByTTL` contains the
provider-reported lifetime breakdown of `CacheWriteTokens`. Do not add those
values to the total again. Missing accounting is not evidence of zero consumption:
some adapters retain zero-valued usage when upstream accounting is omitted.
Additional native usage details are retained in `Metadata["provider_usage"]`
when they cannot be represented by the normalized fields.

An explicit OpenAI Responses request can prewarm eligible input:

```go
resp, err := client.Text().
    Using("openai"). // Registered with WithOpenAIResponses.
    Model("gpt-5.6").
    Prompt(longReusablePrompt).
    ProviderOptions(map[string]any{"max_output_tokens": 0}).
    Generate(ctx)
```

For the proxy, set `"wormhole": {"cache_prewarm": true}` on an OpenAI
Responses-backed route. Unsupported routes fail before HTTP. A successful
prewarm may have empty text and no tool calls; ordinary empty generations still
fail. Prewarm requests bypass Wormhole's local response cache and idempotency
memoization so an upstream request occurs, while normal transport retries
remain enabled.

Prewarming does not prove a cache hit or a billing saving. Provider rules still
govern eligibility and retention; see the
[OpenAI prompt-caching guide](https://developers.openai.com/api/docs/guides/prompt-caching).

## Reasoning generation and summary visibility

`Reasoning.Effort` accepts `none`, `low`, `medium`, and `high`. Explicit `none`
and explicit false values are preserved rather than treated as missing.
`IncludeThoughts *bool` controls returned thought-summary visibility where the
provider supports it. Hiding summaries does not disable reasoning generation.

| Route | Generation controls |
| --- | --- |
| OpenAI Chat | `reasoning_effort` |
| OpenAI Responses | `reasoning.effort` |
| OpenAI-compatible | The profile's configured reasoning dialect |
| Anthropic | Native thinking enablement and budgets; effort uses native effort with adaptive thinking when an explicit budget mode is absent |
| Gemini | Effort uses `thinkingLevel`; budgets use `thinkingBudget`; `none` requests zero budget, subject to model support |

Gemini retains the historical interpretation of `Enabled` as a summary
visibility control. This legacy setting emits an ambiguity warning;
`IncludeThoughts` takes precedence. Prefer the latter for visibility and effort
or budget settings for generation.

Contradictory typed controls, including disabled thinking with a positive
budget, fail before HTTP. Native `ProviderOptions` overrides retain their
existing precedence. If a model rejects the requested controls, Wormhole
returns the provider error and metadata rather than retrying without them.

Replay signed thinking and tool-call thought signatures unchanged on compatible
turns. Signatures must not cross provider boundaries during fallback. Proxy
`thinking` and `chat_template_kwargs` are compatible-route controls; the
proxy-specific `thinking` object is rejected on native Anthropic, Gemini, and
Ollama routes.

## Tools plus JSON

`Structured().Tools(...).ToolChoice(...)` lets an application handle tool turns
before obtaining a schema-conforming final result. `Generate()` returns either
a response with `ToolCalls` and nil `Data`, or a final response with validated
`Data` and `Raw`. Tool turns retain usage, metadata, provider, and finish reason.
Wormhole does not execute those tools.

`GenerateAs()` returns a typed `ToolCallsRequiredError` carrying the response
when application tools must run first. It does not decode that turn into a
zero-valued destination. The following pattern uses `Generate()` to inspect
each turn explicitly; `lookupWeather` is application code returning JSON text:

```go
messages := []types.Message{types.NewUserMessage("What is the weather in Paris?")}
schema := map[string]any{
    "type": "object",
    "properties": map[string]any{"summary": map[string]any{"type": "string"}},
    "required": []string{"summary"},
    "additionalProperties": false,
}
weather := types.Tool{
    Name: "weather",
    Description: "Look up weather for a city",
    InputSchema: map[string]any{
        "type": "object",
        "properties": map[string]any{"city": map[string]any{"type": "string"}},
        "required": []string{"city"},
        "additionalProperties": false,
    },
}
for turn := 0; turn < 8; turn++ { // The application owns its turn limit.
    resp, err := client.Structured().Using("openai").Model("gpt-5.6").
        Messages(messages...).Schema(schema).Mode(types.StructuredModeTools).
        Tools(weather).Generate(ctx)
    if err != nil {
        return err
    }
    if len(resp.ToolCalls) == 0 {
        fmt.Println(resp.Data) // Validated final object; alternatively decode resp.Raw.
        return nil
    }
    messages = append(messages, &types.AssistantMessage{ToolCalls: resp.ToolCalls})
    for _, call := range resp.ToolCalls {
        if call.Name != "weather" {
            return fmt.Errorf("unexpected tool %q", call.Name)
        }
        result, err := lookupWeather(ctx, call.Arguments)
        if err != nil {
            return err
        }
        messages = append(messages, types.NewToolResultMessage(call.ID, result))
    }
}
return fmt.Errorf("tool turn limit reached")
```

For typed decoding, inspect the error with `errors.As` before handling an
ordinary failure:

```go
var required *types.ToolCallsRequiredError
if err := builder.GenerateAs(ctx, &out); errors.As(err, &required) {
    // Handle required.Response.ToolCalls, append results, then request the next turn.
} else if err != nil {
    return err
}
```

Keep signed thinking blocks from the corresponding text response when the
provider requires them for replay. Preserve tool-call signatures by appending
the returned calls rather than reconstructing them from names and arguments.

JSON mode combines native JSON output with local validation. Strict mode
requires native schema enforcement and rejects unsupported combinations.

Local final-result validation supports object schemas with `type` (including
unions), `const`, `enum`, `properties`, `required`, `additionalProperties`,
`items`, `minItems`, `maxItems`, `uniqueItems`, `minLength`, `maxLength`,
`pattern` (Go regular expressions), `minimum`, `maximum`, numeric
`exclusiveMinimum`/`exclusiveMaximum`, `multipleOf`, `allOf`, `anyOf`, `oneOf`,
`not`, and local `$ref` pointers through `$defs` or `definitions`. `$ref` siblings
also apply. Recursive references may descend through properties or array items;
cycles that do not consume data are rejected before the provider request.

Unknown keywords, unsupported assertions (including `format`, `contains`,
`patternProperties`, conditionals and unevaluated-property constraints), boolean
schemas and malformed keyword values are rejected during structured request
preparation rather than silently ignored. Schema annotations `$schema`, `$id`,
`$comment`, `title`, `description`, `default`, `examples`, `deprecated`, `readOnly`
and `writeOnly` do not assert constraints. External references are unsupported.
Native strict mode may impose additional provider restrictions. Schemas are limited to 10 MiB. Schema and data
validation have a depth limit of 128 and a work limit of 100,000 operations;
exceeding either returns a validation error, including inside alternatives.

Tools mode reserves `__wormhole_structured_output` to carry the final value;
application tools cannot use that name. Default/auto choice allows either kind
of call. `none` disables application tools and selects the result tool.
Specific/required choices target application tools. Application calls are
returned unchanged; the result tool's arguments are schema-validated. Mixing
application calls and a final-result call produces an explicit error retaining
the original response.

Streaming remains on the text API and proxy, without a structured-builder
`Stream()` method. Application tool deltas stream normally. Synthetic result
arguments are buffered and validated before final JSON content is emitted;
final JSON does not stream token by token in this fallback. Invalid JSON ends
with an error, not a successful completion.

Proxy `wormhole.structured_mode: "tools"` selects this fallback explicitly.
Combining it with `response_format` requesting native `strict: true` fails;
Wormhole does not silently weaken strict enforcement. Anthropic citations
conflict with strict JSON output. Gemini schema-plus-tool support depends on
the model and continues to use `generateContent`. Consult the
[Anthropic](https://platform.claude.com/docs/en/build-with-claude/structured-outputs)
and [Gemini](https://ai.google.dev/gemini-api/docs/structured-output) native limits.

## Sources and citations

`Sources` describes retrieved references. `Citations` describes which references
support generated output. Retrieval alone does not imply citation. Wormhole
normalizes returned OpenAI Responses annotations, Anthropic citations, and
Gemini grounding/citation metadata. Search and grounding activation remain
provider options; Wormhole does not orchestrate a search.

Sources have response-scoped IDs, native IDs where available, kind, title, URL,
and raw provider data. Citations reference those IDs and may contain an output
range and source location. A `TextRange` uses half-open UTF-8 byte offsets into
the final response text: `[start, end)`. Source locations can retain page or
native character ranges. An output range is absent when the provider's offset
unit is unknown; Wormhole never guesses exact positions by matching text.

Repeated native records are deduplicated within a response. Distinct references
are not merged just because their URLs match. Malformed optional annotations
retain raw data and emit warnings without discarding successful generation;
broken references do not become fabricated citations.

Warning codes include `unsupported_normalization`, `malformed_citation`, and
`ambiguous_legacy_reasoning`. A warning describes a condition encountered in
that response; the absence of warnings is not a claim of complete native parity.

Stream chunks carry additions, including Anthropic `citations_delta` events,
and the terminal chunk carries the complete snapshot. When collecting, treat
the terminal snapshot as authoritative instead of appending it a second time.
The proxy emits representable native annotations and exposes richer normalized
data only through the requested extension.

## Provider-owned file references

Use `types.FileReferenceMedia` for a file already stored with a provider. Set
`Provider` to its registered provider/profile name, `Kind` to `image`,
`document`, `audio`, or `video`, and exactly one of `ID` or `URI`. Set
`MimeType` wherever the native API requires it.

```go
message := types.NewUserMessage("Summarize this document.")
message.Media = []types.Media{&types.FileReferenceMedia{
    Provider: "openai", Kind: "document", ID: "file-your-existing-id",
}}
resp, err := client.Text().Using("openai").Model("gpt-5.6").
    Messages(message).Generate(ctx)
```

| Route | Supported native references |
| --- | --- |
| OpenAI Responses | Document and image file IDs |
| OpenAI Chat | Document file inputs; image file IDs are rejected |
| Anthropic | Image and document file IDs |
| Gemini | Provider URI plus MIME type in `fileData` |
| Ollama | Provider-owned references are rejected |

OpenAI-compatible profiles follow their configured wire dialect. Unsupported
kinds and representations fail explicitly rather than disappearing from the
request. Before each fallback attempt, the registered provider/profile must
match the reference's owner. An opaque file ID is never forwarded to another
provider or removed to make fallback succeed.

The proxy accepts representable standard OpenAI file content and associates
native references with the routed provider. Other routes can use an explicit
content part:

```json
{
  "type": "wormhole_file",
  "provider": "gemini",
  "kind": "document",
  "uri": "gs://your-provider-accessible-bucket/report.pdf",
  "mime_type": "application/pdf"
}
```

This only describes the reference. Wormhole performs no upload, listing,
deletion, local-path reading, URL fetching, or existence probing. Your
application owns file lifetime and access. See native
[OpenAI file inputs](https://developers.openai.com/api/docs/guides/file-inputs)
and [Anthropic Files integration](https://platform.claude.com/docs/en/build-with-claude/files).

## Local profiles

The proxy discovers eligible local OpenAI-compatible profiles through
`KnownProviderProfiles()` and registers each only when its documented base-URL
environment variable is explicitly nonempty. It does not probe ports, start
services, or activate implicit localhost defaults. Configure the endpoint and
use the server's exact model ID as shown in the
[local daemon guide](local-daemon-launchd.md).
