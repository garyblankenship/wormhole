# Wormhole SDK - Gaps & Missing Features

**Date**: 2026-01-14
**Test Coverage**: 52.6% (wormhole core), 47.8% overall

---

## Executive Summary

This document identifies missing features, incomplete implementations, and gaps discovered through comprehensive codebase audit.

**Key Findings**:
- **2 TODOs** in production code
- **4 failing tests** (Gemini streaming)
- **Test coverage gaps** (Ollama 12.6%, types 21.2%)
- **Missing provider implementations** (image/audio for Gemini, Ollama)

---

## CRITICAL Gaps

### 1. Middleware Resource Cleanup Not Implemented

**Location**: `pkg/wormhole/wormhole.go:954`

```go
// TODO: Close middleware resources (circuit breakers, rate limiters)
```

**Impact**: Goroutine/memory leaks in long-running apps with circuit breakers, rate limiters, health checks.

**Fix**: Add `io.Closer` to middleware interface, implement `Close()` in all middleware, call in `Shutdown()`.

---

### 2. Gemini Streaming Tests Failing

**Tests**:
- `TestGeminiProvider_StreamContext/Stream_with_immediate_context_cancellation`
- `TestGeminiProvider_StreamRequestFormat/Stream_request_with_all_parameters`
- `TestGeminiProvider_StreamErrorScenarios/*`

**Error**: `AUTH_ERROR: API key is required for Bearer authentication`

**Root Cause**: Gemini uses URL param auth, tests expect Bearer headers.

**Impact**: Streaming for Gemini may be broken in production.

---

## HIGH Priority Gaps

### 3. Zero Test Coverage for Testing Utilities

`pkg/testing/`: **0.0% coverage**

- Mock provider untested
- TODO at line 23: `_ = mockProvider // TODO: inject into wormhole for actual testing`

---

### 4. Low Test Coverage

| Package | Coverage | Missing Tests |
|---------|----------|---------------|
| Ollama | 12.6% | Embeddings, streaming errors |
| Types | 21.2% | Validation, error helpers |
| OpenAI | 31.8% | Structured output, tools, audio/image |

---

### 5. Image Generation Missing

- ✅ OpenAI: Implemented
- ❌ **Gemini**: Missing (API supports it)
- ❌ **Ollama**: Missing (some models support it)

---

### 6. Audio Operations Incomplete

**TTS**:
- ✅ OpenAI
- ❌ Gemini (API supports it)

**STT**:
- ✅ OpenAI (Whisper)
- ❌ Gemini (API supports it)

---

## MEDIUM Priority Gaps

### 7. Model Discovery Limited for Anthropic

Hardcoded model list instead of API discovery. New models require SDK updates.

---

### 8. BaseURL Override Test Failing

`TestBaseURLFunctionality/BaseURL_changes_target_endpoint` - expects connection refused, gets auth error.

---

### 9. Provider Capability Detection Incomplete

Static, provider-level only. Missing:
- Model-level capabilities (GPT-4o has vision, GPT-3.5 doesn't)
- Dynamic capability detection

---

### 10. Streaming Error Recovery Not Tested

Missing tests for:
- Server closes connection mid-stream
- Malformed SSE events
- Context cancellation during stream

---

### 11. No Batch Request Validation

`BatchBuilder` lacks:
- Empty batch check
- Duplicate detection
- Total token limit validation

---

### 12. Idempotency Cache Memory Leak Risk

Each cached item spawns goroutine for TTL cleanup. 10K requests = 10K sleeping goroutines.

**Fix**: Use periodic sweep with `time.Ticker`.

---

## LOW Priority Gaps

### 13. No Request/Response Logging Hook

Users must write middleware for custom logging.

**Desired**: `WithRequestHook()`, `WithResponseHook()` options.

---

### 14. No Cost Estimation API

Users must manually calculate costs from `Usage` struct.

**Desired**: `usage.EstimateCost("gpt-4o")` helper.

---

### 15. No Provider Health Dashboard

`HealthMiddleware` tracks health but no query API.

**Desired**: `GetProviderHealth()` method.

---

### 16. No Auto-Retry for Structured Output Parsing Failures

If JSON parsing fails, no auto-retry with validation error feedback to model.

---

### 17. Missing Usage Aggregation for Batch Requests

No `TotalUsage()` method to sum tokens across batch results.

---

### 18. No OpenTelemetry Integration

Metrics exist but no distributed tracing spans.

---

### 19. Tool Calling Doesn't Support Parallel Execution

Tools run sequentially even when model requests multiple.

**Desired**: Parallel execution with `sync.WaitGroup`.

---

### 20. No Provider Fallback Chain

No automatic failover from primary to secondary provider.

**Desired**: `WithFallbackChain("openai", "anthropic", "openrouter")`.

---

## Documentation Gaps

### 21. Missing Advanced Examples

- Multi-turn tool calling
- Custom middleware
- Batch with concurrency limits
- Streaming with backpressure

---

### 22. No Migration Guide v1.0 → v1.1

Type-safe middleware introduced but no migration docs.

---

### 23. Provider Capability Matrix Not Documented

README lists providers but not their capabilities table.

---

## Summary

| Category | Count | Priority |
|----------|-------|----------|
| Missing Implementations | 8 | 5 HIGH |
| Test Coverage | 6 | 2 CRITICAL, 4 HIGH |
| API Gaps | 9 | 4 MEDIUM, 5 LOW |
| Documentation | 3 | LOW |
| **TOTAL** | **26** | 2 CRITICAL, 9 HIGH, 7 MEDIUM, 8 LOW |

**Priority Actions**:
1. Fix middleware cleanup (CRITICAL)
2. Fix Gemini tests (CRITICAL)
3. Implement Gemini/Ollama audio/image (HIGH)
4. Improve test coverage (HIGH)

---

**Last Updated**: 2026-01-14
