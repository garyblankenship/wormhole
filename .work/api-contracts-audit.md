# Wormhole API-Contracts Audit — Backlog
**Date:** 2026-07-08 · **Mode:** scoped api-contracts lane (grep-first, adversarially verified) · **Repo state:** post-v1.22.0 (c6f0feb)..HEAD
**Coverage caveat:** `openai/transform.go` was NOT audited — its review agent hit a 429 rate limit. Re-run that one file to close the gap.
**Status legend:** ✅ FIXED (commit) · ⬜ OPEN

## Confirmed Findings

| Severity | File:Line | Category | Summary | Failure Scenario | Status |
|----------|-----------|----------|---------|------------------|--------|
| **HIGH** | `pkg/providers/ollama/transform.go:303` (actual: `transform/streaming.go:209-215,479-488`) | correctness | Ollama streaming transformer marks every intermediate (`done:false`) chunk with FinishReason=Stop, so `IsDone()` returns true on every chunk. | The SDK `Stream()` forwards all chunks (does NOT truncate), but any consumer/accumulator gating on `IsDone()` — including the documented `if chunk.IsDone() { break }` idiom — truncates to one chunk, and the real finish reason / `done_reason` is never surfaced. Raw range-over-channel consumption is unaffected. (Severity corrected from an initial 'critical' overstatement after reading `ollama.go:74-90` stampProvider + `responses.go:153` IsDone.) | ✅ FIXED (13c1b85 + test 00d4c52) |
| **high** | `pkg/providers/ollama/ollama.go:136` | correctness | `Structured()` mutates caller's `UserMessage.Content` in-place by appending schema instructions; shared slice of pointer-typed messages propagates stale schema text on reuse. | Conversation loop reusing the same `[]Message` across multiple `Structured()` calls accumulates stale schema instructions. | ⬜ OPEN |
| **high** | `internal/server/types.go:140` | correctness | `ChatToolCallFunction.Name` lacks `omitempty` — every streaming tool-call continuation delta serializes `"name":""` instead of omitting the field. | Strictly-parsing clients misinterpret continuation fragments as new tool calls. | ⬜ OPEN |
| **high** | `internal/server/types.go:13` | correctness | `ChatCompletionRequest` missing `max_completion_tokens` — newer OpenAI clients sending this field have the token limit silently dropped. | Model runs with default/unbounded output length instead of the requested limit. | ⬜ OPEN |
| **medium** | `pkg/providers/gemini/transform.go:601` | correctness | `transformStructuredResponse` concatenates thinking-part text into JSON parse input, causing `json.Unmarshal` to fail on thinking models. | Gemini 2.5 Flash/Pro structured-output calls with thinking parts always fail with "failed to parse structured response." | ⬜ OPEN |
| **medium** | `pkg/providers/transform/streaming.go:479` | correctness | Ollama streaming transformer ignores `done_reason` field; all streams report `FinishReasonStop` regardless of actual stop reason. | Max-tokens truncation (`done_reason:"length"`) reported as normal completion; caller cannot detect truncation. | ✅ FIXED via `ExtraFinishReasonPath` in b0a37c8 |
| **medium** | `pkg/providers/ollama/transform.go:247` | correctness | `parseStreamChunk` fallback (lines 253-296) is unreachable dead code — `New()` always initializes `streamingTransformer`, so the nil-guard never fires. (downstream of the streaming-transformer path; verify still dead after 13c1b85) | Dead code only; the correct `done_reason` handling in the fallback is never exercised at runtime. | ⬜ OPEN |
| **medium** | `pkg/providers/transform/common.go:28` | correctness | `MapFinishReason` default case maps unknown reasons to `FinishReasonStop` instead of `FinishReasonOther`. | New provider reasons (timeout, rate_limit) silently reported as normal completion. | ⬜ OPEN |
| **medium** | `pkg/providers/transform/common.go:38` | correctness | `TransformTextResponse` omits `FinishReason` from returned `TextResponse`, leaving zero-value empty string. `IsComplete()` returns false for normal completions. | Currently dead code (test-only callers), but the signature is a latent trap for future production use. | ⬜ OPEN |
| **medium** | `pkg/providers/transform/common.go:166` | correctness | `ParseUsageFromMap` drops cache token fields (`CacheReadTokens`, `CacheWriteTokens`); streaming path handles them correctly but this shared helper does not. | Currently dead code (no production callers), but asymmetry is a footgun if the helper gets wired in. | ⬜ OPEN |
| **medium** | `internal/server/types.go:13` | correctness | `ChatCompletionRequest` missing `n` field — multi-choice requests silently degraded to single choice. | Client sending `n:3` expects 3 completions, gets exactly 1 with no error. | ⬜ OPEN |
| **medium** | `internal/server/types.go:13` | correctness | `ChatCompletionRequest` missing `frequency_penalty`, `presence_penalty`, `seed`, `parallel_tool_calls` — standard sampling parameters silently dropped. | Model behaves with defaults instead of requested sampling behavior. | ⬜ OPEN |
| **medium** | `internal/server/types.go:163` | correctness | `ChatUsage` missing `completion_tokens_details` and `prompt_tokens_details` — reasoning token breakdowns invisible to proxy clients. | Clients using o1/o3 or Claude extended thinking cannot see token composition. (Root cause: SDK also never captures reasoning_tokens from OpenAI.) | ⬜ OPEN |
| **medium** | `internal/server/types.go:110` | correctness | `ChatMessage` missing `Name` field — participant/agent names silently dropped. Defect is systemic across all three layers (proxy struct, SDK internal types, Message interface). | Rarely-used field; real but low-priority gap across the full pipeline. | ⬜ OPEN |
| **low** | `pkg/providers/gemini/transform.go:529` | correctness | Safety-blocked prompts return generic "no candidates in response" without checking `promptFeedback.blockReason`. | Consumer cannot distinguish a safety block from an empty response. | ⬜ OPEN |
| **low** | `pkg/providers/gemini/transform.go:99` | correctness | Dead code: `mapRole` maps `"system"` to `"model"`, but system messages are filtered before reaching `mapRole`. | Unreachable today; becomes a real bug if system-message filtering is refactored away. | ⬜ OPEN |
| **low** | `pkg/providers/gemini/types.go:49` | correctness | `usageMetadata` struct missing `thoughtsTokenCount` — thinking token costs silently dropped. Cross-layer gap: `types.Usage` also lacks a `ThinkingTokens` field. | Consumers never see thinking token costs for Gemini 2.5+ responses. | ⬜ OPEN |
| **low** | `pkg/providers/transform/common.go:90` | correctness | `ParseToolCallFromMap` does not capture the `index` field, leaving `Index=0` on all tool calls. | Currently dead code; would corrupt multi-tool-call streaming assembly if wired in. | ⬜ OPEN |
| **low** | `pkg/providers/transform/common.go:236` | correctness | `LenientUnmarshal` performs strict `json.Unmarshal` with no actual lenience; name and doc comment are misleading. | Misleading name could cause a caller to skip their own fallback logic. No production callers today. | ⬜ OPEN |
| **low** | `internal/server/types.go:170` | correctness | `EmbeddingRequest` missing `encoding_format` and `dimensions` — proxy always returns float arrays at full dimensionality. | Client requesting base64 format or reduced dimensions gets float arrays at full dimensionality instead. | ⬜ OPEN |

## Plausible / Unverified

| Severity | File:Line | Summary | Why Uncertain |
|----------|-----------|---------|---------------|
| **high** | `internal/server/types.go:13` | `ChatCompletionRequest` missing `stream_options` field — silently drops client's include_usage request | **Refuted consequence.** The OpenAI provider unconditionally sets `stream_options: {include_usage: true}` on every stream request regardless of client input. Clients do receive streaming usage data; the missing inbound field is structurally real but functionally irrelevant. |
| **medium** | `internal/server/types.go:145` | `ChatCompletionResponse` missing `system_fingerprint` field | **Refuted consequence.** The SDK's normalized `TextResponse` type has no `SystemFingerprint` field — there is no upstream value to propagate even if the proxy struct added it. Cosmetic JSON shape gap, not a data-loss defect. |

## Wire-Conformance Test Coverage

### Provider Matrix

| Provider | Wire Tests | Streaming Tests | Notes |
|----------|------------|-----------------|-------|
| **Anthropic** | Yes (1 golden SSE) | Yes | Single `testdata/stream_tool_use_thinking.sse`. Covers tool_use, thinking, usage, finish_reason. No partial-line or buffer-boundary tests. |
| **Gemini** | Yes (1 golden SSE) | Yes | Single `testdata/stream_tool_call_thinking.sse`. Strong streaming edge-case table (empty chunks, errors, cancellation, partial+close, empty stream). Stream usage tests for top-level, cache read, premature EOF. |
| **OpenAI** | Yes (1 golden SSE) | Yes | Single `testdata/stream_tool_call.sse`. Stream accumulator (parallel tool calls with wire index), cancel, reasoning_content, malformed tool args. Shared transformer tests cover Ollama/Anthropic/OpenAI finish reason mapping. |
| **Ollama** | **No** | Yes (limited) | No wire_conformance_test.go, no testdata/. Only `TestParseStreamChunkFallback` (nil transformer) — malformed JSON, normal chunk, done=true. No NDJSON split-line, empty-line, or buffer-boundary tests. |
| **OpenAI-compatible** | No (shares openai) | No (separate) | Alias of openai provider. No tests for compatible-provider quirks (empty `usage:{}`, non-standard finish_reasons). |

### Missing Wire Conformance

- **Ollama** — no `wire_conformance_test.go`, no `testdata/`, no golden NDJSON replay.
- **OpenAI-compatible** — no provider-specific wire conformance (shares openai structurally but no tests for compatible-provider-specific wire quirks).
- **Proxy server** (`internal/server/`) — no wire conformance golden replay tests; all 4 streaming tests use mock providers, not real-shaped SSE payloads.

### Concrete Coverage Gaps

1. Partial-line SSE buffering: no provider-level test verifies behavior when an SSE `data:` line arrives split across two TCP reads (utility-level coverage exists at `internal/utils/sse_test.go`).
2. Ollama NDJSON edge cases: no tests for split JSON objects across reads, empty lines between objects, or truncated final JSON.
3. Golden-file variety: Anthropic, Gemini, and OpenAI each have only one `.sse` golden file — no replays for pure text streaming, streaming with images/vision, multiple tool calls, or error-shaped SSE streams.
4. Empty `tool_calls` array: no provider-level test for upstream sending `{"tool_calls":[]}` (zero-length array vs. null/missing).
5. Usage-on-final-chunk: no explicit test verifying usage is surfaced only on the terminal chunk (not intermediate chunks).
6. Proxy wire conformance: zero golden-file replay tests through the proxy's SSE re-serialization path.

## Residual Risk

- **Runtime-only behavior not covered by static analysis:** Concurrency races in streaming (goroutine leaks, channel closure timing), HTTP connection lifecycle (timeouts, keepalive, retry behavior under load), and memory pressure from large streaming responses or many concurrent streams are outside this audit's scope.
- **Provider response schema drift:** All findings assume current provider API shapes. Any upstream provider changing field names, types, or semantics (e.g., Ollama renaming `done` to `finished`, Gemini restructuring `usageMetadata`) would introduce new defects not detectable from this snapshot.
- **SDK public API surface:** This audit scoped to internal provider transforms, proxy types, and streaming plumbing. The public SDK API (`pkg/wormhole/` public types, builder methods, agent builder) was not audited for contract completeness or semantic correctness.
- **Authentication and transport:** API key handling, retry logic (`go-retryablehttp` configuration), TLS settings, and error propagation from the transport layer to callers were explicitly excluded.
