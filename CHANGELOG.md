# Changelog

## v2.0.0 (2026-07-14)

### Breaking changes
- Prepare the v2 module and package-layout migration; see the [v2 migration guide](docs/v2-migration.md) for import mappings and removed implementation packages.

### Other
- Move provider stream, retry, multipart, JSON, and discovery cache-path helpers to the packages that own them.
- Deprecate the v1 adapter and implementation-only packages scheduled for removal or internalization in v2.

## v1.26.0 (2026-07-13)

### Features
- Proxy: add an OpenAI-compatible `POST /v1/rerank` endpoint with provider/model routing, `top_n`, and usage reporting.

### Fixes
- Agent tools: preserve tool-call correlation by emitting one result message per tool call, including parallel calls.

### Other
- Add the Codex-through-Z.AI bridge guide and ready-to-copy configuration assets.
- Prune stale documentation, repair retained guides and examples, and restore navigation across the supported docs.
- Stop tracking the ignored local API-contract audit artifact.

## v1.25.0 (2026-07-13)

### Features
- Proxy: complete sequenced Responses streaming lifecycles for text, refusals, function calls, custom tools, and detailed usage.
- Provider configuration: support `APIKeys`-only authentication consistently through runtime requests and model discovery.
- Types: add safe structured logging helpers and detached cloning for mutable request, model, registry, and provider-profile state.

### Fixes
- Security: prevent raw upstream bodies, causes, prompts, credentials, and unbounded provider-controlled metadata from reaching default logs.
- Proxy: reject malformed or undeclared tools and tool choices before provider I/O, preserve refusal content, and map unknown finish reasons to `other`.
- Middleware: isolate circuit breakers by provider and operation so one failed route cannot block healthy fallbacks.
- Providers: normalize tool calls before serialization and stop Gemini and Ollama transformations from mutating caller-owned data.
- Discovery: preserve stale catalogs for ordinary reads, make manual refresh strict and deterministic, and cancel in-flight refreshes promptly on shutdown.
- Registries and builders: detach nested mutable state across clones, getters, registration, and option merging.

### Other
- Resolve the deterministic lint backlog and expand race/regression coverage across logging, circuits, discovery, proxy streaming, provider transforms, and ownership boundaries.

## v1.24.0 (2026-07-11)

### Features
- Proxy: add a Codex-compatible `/v1/responses` bridge with text streaming, function and custom tools, tool continuations, image input, usage, and incomplete-response mapping.
- Text builder: add explicit cross-provider fallback routes for generation and streaming.

### Fixes
- Proxy: preserve custom and `allowed_tools` constraints, map Z.AI reasoning requests to `thinking.type`, emit complete streaming tool lifecycles, and reject malformed tool results locally.
- Discovery: isolate cached model values from caller mutation and prevent provider shard-name collisions while preserving legacy cache migration.
- Streaming and lifecycle: cancel failed provider attempts cleanly, enforce idle timeouts per attempt, and harden shutdown coordination.

### Other
- Add Codex model-list compatibility and expand fallback, lifecycle, discovery-cache, and production-hardening regression coverage.

## v1.23.0 (2026-07-09)

### Features
- Types: add `ToolResultMessage.Error` and `WithError` so tool execution failures can be represented explicitly.

### Fixes
- Gemini: account for hidden thinking tokens by mapping `thoughtsTokenCount` into completion usage and `ReasoningTokens`.
- Gemini: exclude thought parts from structured-output JSON parsing and emit `functionResponse.error` on failed tool results.
- OpenAI: surface `reasoning_tokens` and refusals.
- Server: forward `max_completion_tokens` and embedding dimensions, omit empty tool-call names, and map SDK-internal auth/rate/timeout errors to the correct HTTP status.
- Providers: preserve native generation/finish reasons and serialize tool-call names correctly.
- Anthropic: populate non-streaming tool-use arguments and emit `is_error` on failed tool results.
- Ollama: stop marking intermediate streaming chunks as terminal.
- Adaptive concurrency: default zero `AdjustmentInterval` safely.

### Other
- Restore Gemini transform tests and add Ollama wire-conformance streaming coverage.

## v1.22.0 (2026-07-03)

### Compatibility Notes
- `ProviderMiddleware` implementations must now provide `ApplyRerank`.
- Provider `AuthStrategy` implementations must now provide `ExtractKey`.
- `StreamAndAccumulate` now returns `func() (string, error)` so callers can observe stream-level accumulation errors.

### Features
- Add retryability-aware tool execution so retry loops can avoid duplicating non-retryable tool side effects.
- Add profile-aware OpenAI-compatible provider registration, including OpenRouter image-path defaults.

### Fixes
- Streaming: close Ollama NDJSON streams, report premature EOF, preserve default request deadlines for streaming and audio requests, and make `StreamAndAccumulate` safe for abandoned consumers and concurrent result reads.
- OpenAI: preserve streamed tool-call indexes so parallel tool calls are accumulated independently instead of being merged into one corrupted call.
- Gemini: report prematurely truncated streams, restore keyless custom-gateway operation, and preserve API-key rotation behavior for query-param auth.
- Proxy: resolve OpenRouter `provider/model` routing against the effective default provider and return actionable `400 invalid_request_error` responses for client-side validation failures.
- Middleware: unwrap `WormholeError` through composed middleware, cap provider `Retry-After`, namespace cache entries by provider, isolate cached values from caller mutation, and bound load-balancer health-check goroutines.
- Discovery: preserve account-scoped fallback and migration behavior, prevent migration writes after `Clear`/`Close`, and keep stale shards from reappearing.
- Adaptive concurrency: default partial PID configs instead of freezing capacity with zero output bounds.
- TLS: preserve custom `RootCAs`, `ServerName`, and `CipherSuites` while flooring insecure TLS settings.
- Structured output: clear stale schema marshal errors after a later valid `Schema()` call.
- Idempotency: honor owner request cancellation/deadlines and avoid starting the sweeper when idempotency is disabled.

### Other
- Expand regression coverage across streaming, proxy routing, middleware caching/retry behavior, discovery cache lifecycle, adaptive config defaults, TLS config preservation, and provider profile registration.

## v1.21.0 (2026-06-27)

### Features
- OpenAI/DeepSeek: surface `reasoning_content` into `TextResponse.Thinking` on non-streaming responses and as `Thinking` deltas on streamed chunks, exposing DeepSeek-R1-style reasoning traces through the unified interface.
- OpenAI/DeepSeek: decode `prompt_cache_hit_tokens` into `Usage.CacheReadTokens` so DeepSeek prompt-cache savings are reported.

### Fixes
- Anthropic: guard a nil `Tool.Function` in `transformTools` — proxy-routed tool requests (which populate the top-level `Tool` fields, not `Function`) no longer panic into an opaque HTTP 500.
- Anthropic: emit the correct `tool_choice` wire format (`auto`/`any`/`none`/`{type:tool,name}`) instead of the generic internal shape, fixing structured-output and forced-tool requests that the real Anthropic API rejected.
- Anthropic: set the top-level `ToolCall.Name` on non-streaming tool calls so the proxy emits a populated `function.name` (previously empty).
- Streaming: guard the `StreamAndAccumulate` send against an abandoned consumer (context cancel / early return) and drain the source channel, preventing a goroutine leak.
- Proxy: wait for graceful shutdown to complete before exiting, so the client-pool `Close()` and in-flight request drain actually run on SIGINT/SIGTERM.
- Errors: include the wrapped cause in `RequestError.Details` so the root cause reaches logs and client responses.

### Other
- OpenAI: extract the Structured response decode into an `extractStructuredData` helper and collapse duplicated Responses tool-call stream-chunk construction.
- Docs: add a DeepSeek provider page (V4 models, reasoning passthrough) and retire the stale `deepseek-chat` reference.
- Tests: add `reasoning_content` surfacing and DeepSeek cache-hit usage coverage.

## v1.20.0 (2026-06-24)

### Features
- Embeddings: add `GenerateBatched(ctx, batchSize)` to split large embedding inputs into provider-sized batches, validate provider response indexes, merge usage, and return vectors in caller input order.
- OpenAI-compatible smoke checks: add optional streaming and embeddings checks, provider-options passthrough, and per-check result details.

### Fixes
- Proxy: preserve upstream HTTP errors when a streaming chat request fails before SSE headers are committed; after commit, emit an SSE error payload instead of a misleading `[DONE]`.
- Streaming: surface the original single-model stream error when first-chunk stream fallback has no alternate model to try.

### Other
- Add regression coverage for batched embeddings, extended OpenAI-compatible smoke checks, and proxy streaming error behavior.

## v1.19.0 (2026-06-22)

### Features
- Proxy: OpenAI-compatible tool-call passthrough — accept `tools`/`tool_choice` on requests, return `tool_calls` on responses and as indexed `tool_call` deltas while streaming, and reconstruct inbound assistant tool calls for multi-turn conversations.
- Errors: preserve provider retry timing on `WormholeError` via a new `RetryAfter` field and `WithRetryAfter` setter; add `types.ParseRetryAfterHeader` to normalize `Retry-After` (seconds or HTTP-date) and `x-ratelimit-reset-requests` (seconds or a Go-style duration like `1m26.4s`). `GetRetryAfter` now prefers an explicit provider hint over its code-based defaults.
- Proxy: log a startup warning when `WORMHOLE_API_KEY` is unset and `/v1/` endpoints are served without authentication.
- Proxy: accept OpenAI `response_format` (structured output) and thread it through to OpenAI and OpenAI-compatible providers. Anthropic, Gemini, and native Ollama return a clear `400` instead of silently producing unstructured output — drive structured output for those providers through the SDK.
### Fixes
- Gemini: send the real function name in `functionResponse.name` instead of the synthetic call ID, fixing HTTP 400 on multi-turn tool calling.
- Ollama: guard the streaming `stampProvider` send with `ctx.Done()` to prevent a goroutine leak on cancellation.
- Proxy: compare the API key in constant time and add `ReadTimeout`/`IdleTimeout` for slowloris hardening (`WriteTimeout` is left unset to preserve long-lived SSE streams).
- Proxy: redact upstream provider error details from client responses — the full error still reaches the server logs.
- HTTP: log a failed re-auth during key rotation instead of silently swallowing the error.
- AdaptiveLimiter: `Stop()` now waits for the adjustment goroutine to exit instead of leaking it.

### Other
- Docs: document the proxy tool-call passthrough, auth/redaction, and `response_format` behavior in the README.
- Tests: add proxy tool-call mapper, streaming delta indexing, error-redaction, and Gemini `functionResponse.name` coverage; add `response_format` gate tests; add wire-conformance replay tests (real-payload SSE fixtures driven through the actual parse path) for Gemini, Anthropic, and OpenAI streaming.

## v1.18.0 (2026-06-20)

### Fixes
- OpenAI: honor `StructuredModeStrict` with native `json_schema` structured output (previously a silent downgrade to function-calling) on both Chat Completions and the Responses API
- Gemini: pass structured tool-result JSON through `functionResponse.response` as an object instead of a double-encoded string

### Other
- Raise the minimum Go version to 1.25 (enables `testing/synctest`-based concurrency tests)

## v1.17.0 (2026-06-20)

### Features
- Gemini: preserve and replay thinking-model `thoughtSignature` tokens across multi-turn and cross-provider calls, including a sentinel for Gemini-3
- Gemini: normalize JSON Schema union/nullable types and emit an explicit `type` on every typed-schema node for full structured-output fidelity
- Anthropic: replay signed thinking blocks on multi-turn, guarded to same-provider origin
- Flag malformed tool-call arguments (`ArgsInvalid`) instead of silently swallowing them, via a centralized parser in `pkg/types`
- Add `ValidateMessageSequence` and cross-provider message-sequence repair: orphaned tool calls and stranded tool results are dropped before dispatch

### Fixes
- Anthropic: emit proper `tool_result` blocks for tool messages; guard nil Function on tool-call replay; capture `message_start` usage on streamed chunks; treat 529 `overloaded_error` as retryable; coalesce consecutive same-role messages into one turn
- Gemini: request SSE framing (`alt=sse`) on `streamGenerateContent`; surface top-level `usageMetadata` and `cachedContentTokenCount` on the stream path; route thought parts to Thinking instead of answer text; strip the `google/` model prefix; coalesce consecutive same-role messages
- OpenAI: set `stream_options.include_usage` and capture `cached_tokens` on streamed requests; nil partial args on malformed Responses-API tool calls; deterministically honor a canceled context in the stream send guard
- Proxy: map the `developer` role to system and `function`/`tool` roles to tool-result messages; map upstream `WormholeError` status onto the HTTP response
- Ollama: include `UserMessage` media images in the chat payload; route messages through the `PrepareMessages` repair seam
- Errors: classify `insufficient_quota` 429 as a quota error rather than a retryable rate-limit; make auth errors non-retryable; surface provider error type/code in `Details`
- Streaming: raise the SSE scanner buffer above 64KB to avoid truncating large events; don't let an empty trailing usage frame clobber real usage
- Embeddings: backfill response `Model` from the request for OpenAI and Gemini

### Other
- Centralize tool-call argument parsing and the `ArgsInvalid` contract in `pkg/types/tool_args.go`
- Expand provider regression coverage (Anthropic, Gemini, Ollama, OpenAI), including golden round-trips for `thoughtSignature` passthrough and message-sequence repair

## v1.16.1 (2026-06-19)

### Features
- add first-class no-auth local OpenAI-compatible setup and smoke diagnostics
- add cross-provider message-sequence repair: orphaned tool calls and stranded tool results are silently repaired (and tool-call IDs normalized) before provider dispatch

### Fixes
- preserve low-level network causes in top-level provider errors
- skip OpenAI key-prefix validation for custom OpenAI-compatible base URLs

### Other
- document local-compatible base URL expectations and retry behavior

## v1.16.0 (2026-06-14)

### Features
- add provider-neutral reasoning controls for OpenAI, Anthropic, and Gemini text requests
- add profile-backed request policy for provider/model token parameter quirks

### Other
- expand provider option and response streaming regression coverage

## v1.14.0 (2026-06-09)

### Features
- add Gemini image generation support with native `generateContent` image responses

### Other
- update package metadata files
- update package tests

## v1.13.0 (2026-06-04)

### Features
- add per-chunk stream idle timeout via `WithStreamIdleTimeout` option and watchdog wrapper
- add stream lifecycle trace events via `StreamTraceFunc` and `WithStreamTrace` (started/ended/error)
- derive proxy route prefixes from `provider_profiles.json` data instead of hardcoded provider lists

### Fixes
- repair tool-call conversations at the `PrepareMessages` boundary — UTF-8 sanitization, ID synthesis, duplicate detection

### Other
- split anthropic transform into focused files (messages, response, stream, tools) and extract gemini system-message handling; broaden test coverage

## v1.10.0 (2026-05-29)

### Features
- add context-aware model discovery methods and guard background refresh startup
- harden proxy server request handling with capped bodies, context propagation, and flexible embedding input forms

### Fixes
- preserve Anthropic system messages sent through normal message lists
- merge partial discovery config with defaults and add explicit discovery opt-outs
- make proxy model listing use configured and discoverable providers
- make load-balancer health-check shutdown and test mock providers race-safe
- restore local lint, example, and release verification targets
- update CLI version reporting to use build metadata

### Other
- refresh GoReleaser config for current syntax and release paths
- tune lint policy and clean existing lint findings
- format existing Go files touched by verification

## v1.9.2 (2026-05-16)

### Other
- trim non-library files from generated source archives

## v1.9.1 (2026-05-16)

### Fixes
- remove local `.work` scratch files from the published module

## v1.9.0 (2026-05-16)

### Features
- add OpenAI-compatible proxy server with model prefix routing
- add agentic loop primitive with scoped tool registry

### Fixes
- wire OpenAI image generation capability through the public image builder
- harden orchestration usage handling

### Other
- refresh project README and banner
- prune unused indirect modules
- expand deterministic test coverage
- align provider and guide examples
- document proxy server usage, installation, and model prefix routing
- document agent loop with usage examples
- update model names in package docs to current versions
- fix API signatures, import paths, broken links, and remove doc test files
- move scratch files to `.work` and normalize doc filenames to lowercase
- update copyright year and Makefile comment
