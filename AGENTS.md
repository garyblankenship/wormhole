# Wormhole SDK

Go SDK for unified LLM provider access.

## Scope: Provider Bridge (BLOCKING)

Wormhole is a **provider bridge / gateway** — a unified façade over LLM providers, deliberately NOT a reimplementation of every provider's full feature set. This scope is a design constraint, not an accident. Keep it focused; do not let it drift into a do-everything abstraction layer.

**In scope — the "app-facing path"** (the real-time tasks an app uses):
- Text generation, streaming
- Structured JSON output
- Tool calling
- Embeddings
- Reranking (OpenAI-compatible `/rerank`, e.g. OpenRouter `cohere/rerank-v3.5`)
- The standalone OpenAI-compatible **proxy** form (`provider/model` prefix routing, e.g. `anthropic/claude-sonnet-4-5`)

**Out of scope — platform administration** (point users to the official provider SDK/REST API; do NOT rebuild it here):
- OpenAI Vector Stores, Assistants, files, fine-tuning
- Anthropic Message Batches, files
- Provider-specific fine-tuning jobs and resource management
- Realtime/admin endpoints

**Gate for any new feature:** ask "everyday app generation, or platform administration?" If admin → decline and cite the official SDK. Rebuilding every provider's full feature set is "how a normal Thursday becomes a three-week expedition." User-facing statement of this lives in `README.md` (intro + "What The Portal Does Not Do").

## Model Reference URLs

| Provider | Models List URL |
|----------|-----------------|
| **Google Gemini** | https://ai.google.dev/gemini-api/docs/models |
| **OpenAI** | https://platform.openai.com/docs/models |
| **Anthropic** | https://docs.anthropic.com/en/docs/about-claude/models/overview |
| **OpenRouter** | https://openrouter.ai/models |

---

## Current Models (January 2026)

### OpenAI

**GPT-5.2 Family (Latest)**
- `gpt-5.2`
- `gpt-5.2-chat-latest`
- `gpt-5.2-pro`

**GPT-5.1 Family**
- `gpt-5.1`
- `gpt-5.1-mini`
- `gpt-5.1-chat-latest`
- `gpt-5.1-codex-max`
- `gpt-5.1-codex`
- `gpt-5.1-codex-mini`

**GPT-5 Family**
- `gpt-5`
- `gpt-5-mini`
- `gpt-5-nano`
- `gpt-5-2025-08-07`
- `gpt-5-mini-2025-08-07`
- `gpt-5-nano-2025-08-07`

**O-Series Reasoning**
- `o3`
- `o3-pro`
- `o3-mini`
- `o3-deep-research`
- `o4-mini`
- `o4-mini-deep-research`
- `o1`
- `o1-pro`
- `o1-2024-12-17`

**GPT-4.1 Family**
- `gpt-4.1`
- `gpt-4.1-mini`
- `gpt-4.1-nano`
- `gpt-4.1-2025-04-14`
- `gpt-4.1-mini-2025-04-14`
- `gpt-4.1-nano-2025-04-14`

**GPT-4o Family (Legacy)**
- `gpt-4o`
- `gpt-4o-mini`
- `gpt-4o-audio-preview`
- `gpt-4o-mini-audio-preview`
- `gpt-4o-transcribe`
- `gpt-4o-mini-transcribe`
- `gpt-4o-search-preview`

**Audio/Realtime**
- `gpt-realtime`
- `gpt-realtime-mini`
- `gpt-audio`
- `gpt-audio-mini`
- `gpt-4o-mini-tts`

**Image & Video**
- `gpt-image-1`
- `gpt-image-1-mini`
- `sora-2`
- `sora-2-pro`

**Open-Weight**
- `gpt-oss-120b`
- `gpt-oss-20b`

**Embeddings**
- `text-embedding-3-large`
- `text-embedding-3-small`

---

### Anthropic Claude

**Claude 4.5 Family (Latest)**
- `claude-sonnet-4-5-20250929` (alias: `claude-sonnet-4-5`)
- `claude-haiku-4-5-20251001` (alias: `claude-haiku-4-5`)
- `claude-opus-4-5-20251101` (alias: `claude-opus-4-5`)

**Claude 4.x Family (Legacy)**
- `claude-opus-4-1-20250805` (alias: `claude-opus-4-1`)
- `claude-sonnet-4-20250514` (alias: `claude-sonnet-4-0`)
- `claude-opus-4-20250514` (alias: `claude-opus-4-0`)

**Claude 3.x Family (Legacy)**
- `claude-3-7-sonnet-20250219` (alias: `claude-3-7-sonnet-latest`)
- `claude-3-5-haiku-20241022` (alias: `claude-3-5-haiku-latest`)
- `claude-3-haiku-20240307`

---

### Google Gemini

**Gemini 3 Series (Latest)**
- `gemini-3-pro-preview`
- `gemini-3-pro-image-preview`

**Gemini 2.5 Series**
- `gemini-2.5-flash` (stable)
- `gemini-2.5-flash-preview-09-2025`
- `gemini-2.5-flash-image`
- `gemini-2.5-flash-native-audio-preview-12-2025`
- `gemini-2.5-flash-native-audio-preview-09-2025`
- `gemini-2.5-flash-preview-tts`
- `gemini-2.5-flash-lite`
- `gemini-2.5-flash-lite-preview-09-2025`
- `gemini-2.5-pro`
- `gemini-2.5-pro-preview-tts`

**Gemini 2.0 Series**
- `gemini-2.0-flash`
- `gemini-2.0-flash-001`
- `gemini-2.0-flash-exp`
- `gemini-2.0-flash-preview-image-generation`
- `gemini-2.0-flash-lite`
- `gemini-2.0-flash-lite-001`

---

## Development Notes

- Use `go build ./...` to verify changes
- Run `go test ./pkg/wormhole/... -short` for quick validation
- Model names in README.md examples should use current stable versions
- Prefer aliases (e.g., `claude-sonnet-4-5`) over dated versions for examples
- Proxy server lives in `internal/server/` (types, router, server, handler) + `cmd/wormhole/` (CLI)
- Model prefix routing: `provider/model` in the request → strips prefix, routes to that provider; unprefixed → default provider
- Zero new dependencies: proxy uses stdlib only (`net/http`, `encoding/json`, `log/slog`)
- Agent builder lives in `pkg/wormhole/agent_builder.go` + `wormhole_agent.go` — scoped tool registry merges with global, agent tools override globals
- `AgentAddTool` is a package-level generic function (Go disallows generic methods on structs)
