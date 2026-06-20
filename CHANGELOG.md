# Changelog

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
