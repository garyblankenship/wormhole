# SDK tour (offline, deterministic)

Run from the repository root:

```sh
mkdir -p .work/sdk-tour
GOWORK=off go build -o .work/sdk-tour/tour ./examples/sdk-tour
GOWORK=off go build -o .work/sdk-tour/proxy ./cmd/wormhole
WORMHOLE_PROXY_BIN="$(pwd)/.work/sdk-tour/proxy" .work/sdk-tour/tour all
GOWORK=off go run ./examples/sdk-tour list
GOWORK=off go run ./examples/sdk-tour agent
GOWORK=off go run ./examples/sdk-tour native-gemini-structured
```

The default is `all`. A failing scenario exits nonzero after closing its own
client and loopback fixture server. Ctrl-C cancels the current workflow and
runs cleanup. No user credentials, new dependencies, user configuration, or
provider credits are needed. In-process clients disable discovery file caching
and background refresh. Factory/proxy child defaults use disposable HOME folders,
which are removed during cleanup; outbound misroutes are rejected by a local proxy.
The synthetic model IDs are fixture labels, **not deployment recommendations**.

## Examples by workflow

Each named scenario is independently runnable; source files show the real public
SDK builder chains, how to inspect results, propagate context, and close clients.
The local upstream checks requests and supplies independently defined fixture
content. The example then checks exact results, not merely nonempty output.

| Scenario(s) | What the user learns / expected evidence |
| --- | --- |
| `text`, `conversation-vision` | Generation, usage/source attribution, ordered conversation/system messages, exact inline image bytes |
| `sampling-reasoning` | Cloned builders, sampling, seed, parallel tools, reasoning effort and provider-option serialization |
| `stream`, `accumulate` | Consume every chunk, inspect errors after completion, accumulate content, observe exactly one terminal trace |
| `stream-error`, `stream-idle`, `incomplete-output` | Reject truncated/stalled streams and empty budget-exhausted generation |
| `structured-default/json/tools/strict` | Generated struct schemas, typed output, all structured modes, TopP/reasoning/provider options checked on wire |
| `schema-helpers` | Fluent schema construction, local required-field validation, `ContentAs` |
| `embeddings/base64/batched` | Input ordering, exact vectors, base64 little-endian float32 fidelity and sub-batching |
| `rerank` | Query/documents, original document attribution, descending scores |
| `image`, `speech-to-text`, `text-to-speech` | Image bytes, multipart audio upload, exact transcription and speech bytes/format |
| `tools-manual`, `tools-auto` | Typed registration, manual tool/result continuation, automatic handler execution, unregister |
| `agent`, `tool-safety` | Scoped override of global tools, callbacks/history, invalid argument rejection, step-limit evidence |
| `fallback`, `provider-fallback`, `retry`, `key-rotation` | Exact model/provider routes, attempt trace, transient recovery, dummy-key rotation on 429 |
| `batch/collect/first` | Ordered, collect, first-success completion modes and adaptive admission |
| `idempotency`, `handles-shutdown` | Detached cached responses, effective-provider scope, cache clear, provider leases/eviction, shutdown rejection |
| `middleware`, `circuit-isolation`, `cancel-timeout`, `health` | Cache, timeout, rate limit, circuit breaker, typed/enhanced metrics, logging, cancellation/deadlines, health callbacks |
| `catalog`, `model-validation` | Live fixture catalog/refresh, source attribution, model query/capabilities, local validation before HTTP |
| `profiles-local`, `custom-testing` | All compatible profiles, local no-auth/BaseURL override, custom factories and `wormholetest.MockProvider` |
| `responses/stream/structured` | OpenAI Responses instead of Chat Completions, streaming and typed output |
| `native-{anthropic,gemini,ollama}-{text,stream,structured}` | Native protocol adapters with provider-specific completion framing |
| `native-{gemini,ollama}-embeddings`, `native-gemini-image` | Native embeddings and Gemini inline image output |
| `factories-env` | Isolated factory, environment, local, quick text/chat/stream workflows and worker cleanup |
| `proxy` | Built CLI: authentication, chat/stream/Responses, embeddings, rerank, catalog refresh and prefix stripping; Chat reasoning checked on wire in both modes, invalid effort rejected with HTTP 400 |

For grouped entries, run `list` to see the exact hyphenated names.

## Source layout

- `tour.go`: client construction, data loading, scenario lifecycle and output.
- `scenarios.go`: explicit feature inventory.
- Other example files: workflow chains and contract assertions.
- `upstream*.go`: bounded local provider fixtures and request inspection.
- `fixtures/config.json`: prompts, synthetic models, representative inputs,
  expected content, vectors, document scores, binary media and demonstration
  controls. Adjust those data values instead of embedding deployment config in Go.
- `tour_test.go`: parallel integration regressions; `-short` skips loopback runs.

## Limits

These examples exercise the SDK and native HTTP serialization/parsing, not real
model quality, account permissions, cost, latency, or production performance.
The PNG/WAV responses are tiny fixed media fixtures; they are not generated by a
model. Intentional failure scenarios count as clean only when the promised error
is observed. Factory/environment shortcuts run in an isolated child with disposable HOME;
the proxy scenario requires `WORMHOLE_PROXY_BIN` pointing to the built CLI.
Without those binary environment variables, package tests explicitly skip the
two process integrations; the built tour exercises both. No live-provider or
performance verification is implied.

To use a real provider, replace fixture configuration with your application's
provider config and approved model from its catalog; preserve context,
chunk-error inspection and client cleanup. See the main README's provider and
configuration guides.
