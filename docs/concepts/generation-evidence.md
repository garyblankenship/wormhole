# Generation evidence and caller-owned HTTP clients

Wormhole owns provider construction and wire response fidelity. Applications own
credentials, request authority, semantic acceptance, pricing, and settlement.

`openai.NewWithHTTPClient(config, client)` and
`openai.NewWithNameAndHTTPClient(name, config, client)` accept a
`providers.HTTPClient`. All request methods use that client. Passing nil retains
the existing secure default. An injected client and transport remain caller-owned:
provider `Close` does not close them. Injection does not change retry settings;
set `ProviderConfig.MaxRetries` explicitly when using a one-attempt transport.

The optional `types.TextEvidenceProvider` capability is implemented by the concrete
OpenAI-compatible provider. It supports nonstreaming Chat Completions only;
Responses API and streaming requests fail before dispatch. It does not add a
required method to `types.Provider`, a builder fallback, or orchestration.

```go
noRetries := 0
config := types.ProviderConfig{MaxRetries: &noRetries}
provider := openai.NewWithNameAndHTTPClient("internal", config, transport)
result, err := provider.TextWithEvidence(ctx, request)
// Inspect result even when err != nil: a complete body can fail normalization.
if result != nil && result.Evidence.Complete {
    body := result.Evidence.RawBody // owned copy; untrusted, never implicitly logged
    _ = body
}
_ = err
```

`TextWithEvidence` invokes the supplied client once without provider retries.
Both the ordinary normalized response and evidence derive from that one bounded
body (maximum 32 MiB). A transport or incomplete/oversized body error never marks
`Complete`. A completely read body can retain `Complete` even when JSON decoding,
normalization, or HTTP status fails; this certifies completeness only.

The envelope identifies the provider and `chat_completions` API family, retains
original body bytes and response metadata, and exposes every choice's original
role, raw content/refusal JSON, finish reason, and function argument text. The
normalized response keeps existing convenience behavior, including fenced-JSON
cleanup. Unknown continuation fields survive in `RawBody`; this grants no replay
authority.

`RawUsage` retains the original usage JSON. Nullable counts populate only for
nonnegative JSON integers representable by int64: explicit zero is zero; absent,
null, fractional, negative, string, and overflowing values are unknown. Raw JSON
distinguishes these cases. Counts are never inferred from other counts. Evidence
may contain credentials or sensitive generated content; do not automatically log
or forward it.

Use the existing custom-provider factory to register an injected provider:

```go
client := wormhole.New(wormhole.WithCustomProvider("internal",
    func(config types.ProviderConfig) (types.Provider, error) {
        return openai.NewWithNameAndHTTPClient("internal", config, transport), nil
    }))
_ = client
```

For typed evidence, retain or obtain the concrete provider and assert
`types.TextEvidenceProvider` explicitly. Middleware and wrappers are not promised
to forward this optional capability.
