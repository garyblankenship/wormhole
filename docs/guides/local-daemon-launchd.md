# Running Wormhole as a Local macOS Daemon

A hardened `launchd` deployment that keeps the wormhole proxy running across
reboots, scoped to loopback, with real client authentication.

This guide corrects an earlier operational package that was shared against this
repo. The corrections are inline in [Design corrections](#design-corrections)
below; read them if you are migrating from that package.

## What this gives you

- A persistent local OpenAI-compatible endpoint at `http://127.0.0.1:4000/v1`.
- Reboots safely and survives login sessions via `launchd`.
- Real bearer-token authentication on every `/v1/` route, so only clients that
  know the proxy secret can spend your provider credits.
  - `/health` is intentionally unauthenticated (it is a liveness probe only).

## Prerequisites

1. A built `wormhole` binary on your `PATH`. From the repo root:

   ```bash
   go build -o /usr/local/bin/wormhole ./cmd/wormhole
   wormhole --version
   ```

   macOS may block the binary on first run; allow it in *System Settings →
   Privacy & Security*.

2. Provider API keys. This guide uses Anthropic as the default provider.
   Additional providers are configured automatically from `provider_profiles.json`
   when their `*_API_KEY` environment variable is set (see [Selecting providers
   and models](#selecting-providers-and-models)). DeepSeek is one such provider
   (an `openai-compatible` profile keyed on `DEEPSEEK_API_KEY`), so it routes
   natively as `deepseek/<model>` once that key is set.

3. A generated proxy secret (distinct from your provider key). This is the value
   clients send as their bearer token:

   ```bash
   python3 -c "import secrets; print('wh-' + secrets.token_urlsafe(32))"
   ```

   Save it somewhere safe; you will paste it into both the plist and each
   client. Below it is shown as `wh-PROXY-SECRET-CHANGE-ME`.

## 1. The hardened `launchd` deployment

The script writes the plist with an unquoted heredoc so `$HOME` expands to an
absolute path (launchd requires absolute paths and does not expand `~`).
`ThrottleInterval` bounds respawn rate, and `KeepAlive` keeps it up.

Note: `ThrottleInterval` only matters if the wormhole **process exits**. The
proxy does not exit on upstream auth failures — a bad provider key returns HTTP
401 to the client while the daemon keeps running. The throttle is belt-and-
suspenders, not the primary protection.

```bash
set -euo pipefail

# --- Fill these in ----------------------------------------------------------
ANTHROPIC_API_KEY="sk-ant-api03-paste_real_key_here"
DEEPSEEK_API_KEY="sk-deepseek-paste_real_key_here"   # optional; enables deepseek/* routing
WORMHOLE_API_KEY="wh-PROXY-SECRET-CHANGE-ME"          # from step 3 above
# ----------------------------------------------------------------------------

PLIST="$HOME/Library/LaunchAgents/com.wormhole.serve.plist"

mkdir -p "$HOME/Library/LaunchAgents"
mkdir -p "$HOME/Library/Logs/wormhole"

cat > "$PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.wormhole.serve</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/wormhole</string>
        <string>serve</string>
        <string>-addr</string>
        <string>127.0.0.1:4000</string>
        <string>-default-provider</string>
        <string>anthropic</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>ThrottleInterval</key>
    <integer>10</integer>
    <key>StandardOutPath</key>
    <string>$HOME/Library/Logs/wormhole/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>$HOME/Library/Logs/wormhole/stderr.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
        <key>WORMHOLE_API_KEY</key>
        <string>$WORMHOLE_API_KEY</string>
        <key>ANTHROPIC_API_KEY</key>
        <string>$ANTHROPIC_API_KEY</string>
        <key>DEEPSEEK_API_KEY</key>
        <string>$DEEPSEEK_API_KEY</string>
    </dict>
</dict>
</plist>
PLIST

# Restrict the plist: it contains plaintext secrets.
chmod 600 "$PLIST"

# Prefer the modern launchctl subcommands (load/unload are deprecated).
DOMAIN="gui/$(id -u)"
launchctl bootout "$DOMAIN/com.wormhole.serve" 2>/dev/null || true
launchctl bootstrap "$DOMAIN" "$PLIST"
launchctl enable "$DOMAIN/com.wormhole.serve"
launchctl kickstart -k "$DOMAIN/com.wormhole.serve"

sleep 1
launchctl print "$DOMAIN/com.wormhole.serve" | grep -E '"state"|"last exit code"'
echo "---"
curl -s http://127.0.0.1:4000/health
echo
```

Verify it is answering and rejecting unauthenticated traffic:

```bash
# Should return 401
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:4000/v1/models

# Should return 200
curl -s -o /dev/null -w "%{http_code}\n" \
  -H "Authorization: Bearer wh-PROXY-SECRET-CHANGE-ME" \
  http://127.0.0.1:4000/v1/models
```

Tail logs if something is wrong:

```bash
tail -f "$HOME/Library/Logs/wormhole/stderr.log"
launchctl print "gui/$(id -u)/com.wormhole.serve" | grep -A2 "last exit code"
```

To uninstall:

```bash
launchctl bootout "gui/$(id -u)/com.wormhole.serve"
rm "$HOME/Library/LaunchAgents/com.wormhole.serve.plist"
```

## 2. Client connection settings

Clients must send the **real** proxy secret (`WORMHOLE_API_KEY`), not a
placeholder. The proxy rejects `/v1/` requests whose `Authorization: Bearer
<token>` does not constant-time-match that secret.

| Application | Base URL | API key to use | Target model |
| --- | --- | --- | --- |
| **Cursor / VS Code (Continue, etc.)** | `http://127.0.0.1:4000/v1` | `wh-PROXY-SECRET-CHANGE-ME` | `claude-sonnet-4-5` |
| **Open WebUI / Jan / custom OpenAI-compatible UIs** | `http://127.0.0.1:4000/v1` | `wh-PROXY-SECRET-CHANGE-ME` | Select from `/v1/models` dropdown |
| **Custom scripts** | `http://127.0.0.1:4000/v1` | `Authorization: Bearer wh-PROXY-SECRET-CHANGE-ME` | Exact provider/model string, e.g. `anthropic/claude-sonnet-4-5` |

Example:

```bash
curl -s http://127.0.0.1:4000/v1/chat/completions \
  -H "Authorization: Bearer wh-PROXY-SECRET-CHANGE-ME" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-5",
    "messages": [{"role":"user","content":"ping"}]
  }'
```

### Model prefix routing

Wormhole's proxy strips a known provider prefix from the model string and
routes to that provider. Unprefixed models go to `-default-provider`.

- `claude-sonnet-4-5` → routes to the default provider (`anthropic`).
- `anthropic/claude-sonnet-4-5` → routes to Anthropic explicitly.
- `gemini-2.5-pro` → routes to default provider.
- `gemini/gemini-2.5-pro` → routes to Gemini explicitly.
- `openai/gpt-5.2` → routes to OpenAI explicitly.

### Selecting providers and models

The four hardcoded providers are `anthropic`, `gemini`, `ollama`, and `openai`.
Beyond those, `provider_profiles.json` declares a set of `openai-compatible`
providers (DeepSeek, Groq, Mistral, OpenRouter, ZAI, ...) that the proxy wires
up automatically via `WithAllProvidersFromEnv` whenever their `*_API_KEY` is set
in the plist's `EnvironmentVariables`. Each registers under its profile name, so
it becomes a valid model-string prefix.

With `DEEPSEEK_API_KEY` set (as in the plist above), DeepSeek routes directly:

```json
{ "model": "deepseek/deepseek-chat" }
{ "model": "deepseek/deepseek-reasoner" }
```

The same applies to OpenRouter if you set `OPENROUTER_API_KEY`:

```json
{ "model": "openrouter/deepseek/deepseek-chat" }
{ "model": "openrouter/anthropic/claude-sonnet-4-5" }
```

To enumerate the providers actually wired into your running daemon, query the
model list:

```bash
curl -s http://127.0.0.1:4000/v1/models   -H "Authorization: Bearer wh-PROXY-SECRET-CHANGE-ME" | jq '.data[].id' | head
```

Use the model reference URLs in the repository root `AGENTS.md` / `README.md`
for current model names; the examples above use stable aliases.

## 3. System instructions for LLM clients

The earlier shared package framed these rules as necessary because of
"provider failover during a request." That framing is incorrect for this proxy:
routing is **deterministic per request** based on the model string in the
request body. There is no mid-stream provider switching. If you want a style
block, frame it as ordinary client hygiene, not proxy behavior:

```text
You are a software engineering assistant.

1. Output discipline
   - Lead with the answer or the technical deliverable. Drop conversational
     filler ("Here is your code", "Certainly!", "As an AI...").
   - When modifying code, provide targeted diffs, replacement functions, or
     clearly bounded blocks rather than re-emitting entire unmodified files.

2. Reasoning vs. deliverable
   - For multi-step work, separate analysis from the final deliverable.
   - Enclose all executable code, shell commands, and config in fenced,
     language-tagged Markdown blocks.
   - Do not append unsolicited summary paragraphs unless flagging a concrete
     bug, security issue, or breaking change.

3. Environment
   - Assume a POSIX/macOS environment unless told otherwise.
   - Prefer deterministic, idiomatic solutions and the standard library over
     heavy external dependencies.
```

## Design corrections (relative to the prior package)

If you are migrating from the earlier operational package, these are the
reasons each change was made. They are grounded in the live proxy code:

1. **Added `WORMHOLE_API_KEY`.** Without it, `internal/server/server.go`
   disables auth entirely and logs a startup warning; any local process can
   reach the proxy. The "placeholder key" pattern in the earlier package was
   insecure because the proxy never compared it against anything. Now clients
   send the real secret, validated via constant-time compare in the `auth`
   middleware.

2. **Kept DeepSeek, corrected the framing.** DeepSeek is registered as an
   `openai-compatible` provider in `provider_profiles.json` with `auto_env: true`
   and `api_key_env: ["DEEPSEEK_API_KEY"]`. When `DEEPSEEK_API_KEY` is set in
   the daemon's environment, `WithAllProvidersFromEnv` calls
   `WithProviderFromEnv("deepseek")`, which wires it as a routable provider.
   The original package happened to include the right env var, but it described
   the proxy as a "hybrid Anthropic/DeepSeek failover gateway," which it is
   not — it is deterministic per-request routing. Models are addressed as
   `deepseek/<model>`.

3. **Removed the Claude Code CLI client row.** Wormhole serves OpenAI-compatible
   routes (`/v1/chat/completions`, `/v1/responses`, `/v1/embeddings`,
   `/v1/rerank`, `/v1/models`) plus `/health`. It does not serve the
   Anthropic-native `/v1/messages` route that Claude Code CLI requires.

4. **Updated target model to `claude-sonnet-4-5`.** `claude-3-5-sonnet` is
   legacy per the repo's model reference.

5. **Switched to `launchctl bootstrap`/`bootout`.** `load`/`unload` are
   deprecated since OS X 10.10; the modern subcommands also give cleaner state
   via `launchctl print`.

6. **Dropped the "failover/load-based switching" justification from the system
   prompt.** Routing is deterministic per request, decided by the model prefix
   in `parseModelRoute`. The style rules stand on their own as client hygiene.
