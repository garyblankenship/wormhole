# Discovery and provider conformance contracts

Wormhole discovery supports app-facing model selection. It does not manage provider resources or maintain a complete model catalog.

## Configured discovery

The SDK passes OpenAI, Anthropic, and OpenRouter provider configuration to discovery. An explicit `ProviderConfig.BaseURL` takes precedence over the provider's environment override and profile default. Discovery appends `/models`, preserving the configured path prefix.

The additive fetcher constructors `NewOpenAIFetcherWithConfig`, `NewAnthropicFetcherWithConfig`, and `NewOpenRouterFetcherWithConfig` accept `types.ProviderConfig`. They copy custom headers and use the effective API key. Generated authentication is applied before custom headers, so explicit authentication headers take precedence. `NoAuth` suppresses generated discovery authentication; explicitly supplied headers still apply. Keyless OpenAI and Anthropic discovery is registered when `NoAuth` is enabled. This discovery option does not change provider generation authentication implementations.

Configured fetchers scope cache identity to the endpoint, authentication mode, effective key, and canonicalized custom headers. The discriminator is a SHA-256 digest, not a raw URL or credential. Header input changes after construction do not alter the fetcher. Existing constructor signatures and their legacy cache identities remain available.

SDK wiring uses the configured identities, so an existing installation may experience a cold cache miss. Old cache files are neither migrated nor deleted. No operational cache cleanup is required by this change.

## Metadata and selection

`ModelInfo.ContextLength` describes the context window; `MaxTokens` describes the output limit. Zero means unknown. OpenRouter's `context_length` populates `ContextLength`; it does not imply an output limit. Anthropic discovery advertises streaming but leaves unavailable numeric limits at zero.

Positive `ModelQuery.MinContextLength` and `MinMaxTokens` constraints exclude models whose corresponding limits are unknown. Discovery does not infer those limits from model names.

## Freshness and diagnostics

`ListAvailableModelsWithStatus(ctx, provider)` returns `(*discovery.ModelsResult, error)`. Its `Stale` flag distinguishes stale or fallback data from fresh cached or fetched data. Existing model-list methods remain available.

`SelectModelsWithDiagnostics(ctx, query)` returns `(*ModelSelectionResult, error)`:

```go
result, err := client.SelectModelsWithDiagnostics(ctx, wormhole.ModelQuery{
    Capabilities: []types.ModelCapability{types.CapabilityStream},
})
if result != nil {
    for _, diagnostic := range result.Diagnostics {
        // Provider, Stale, and Err describe each attempted provider.
        _ = diagnostic
    }
}
// Handle err and use result.Models when appropriate.
```

Diagnostics are available for attempted provider reads even when selection fails. Errors before any provider read, such as disabled discovery, return a nil result. Existing `SelectModels` and `SelectModel` preserve filtering, ordering, limits, and partial-success behavior. A stale fallback may have a nil error: diagnostics expose the existing discovery result, not a history of hidden refresh failures. Errors are intended for application handling and may contain provider details; they are not automatically published in proxy headers.

Normal proxy `/v1/models` responses include both decimal count headers, including zero:

| Header | Meaning |
| --- | --- |
| `X-Wormhole-Discovery-Failed-Count` | Provider discovery attempts returning an error |
| `X-Wormhole-Discovery-Stale-Count` | Successful provider results marked stale |

HTTP status and model-list JSON remain unchanged, including partial results and empty lists when every provider fails. Headers contain counts only. The existing `client_version` response branch is unchanged.

## Opt-in provider conformance

Custom provider tests can enable additional checks independently:

```go
wormholetest.RunProviderConformance(t, wormholetest.ProviderConformanceConfig{
    Provider:                          provider,
    TextModel:                         "fixture-model",
    ToolModel:                         "fixture-tool-model",
    RerankModel:                       "fixture-rerank-model",
    CheckToolCalling:                  true,
    CheckRerank:                       true,
    CheckStreamCancellationAfterStart: true,
    Timeout:                          time.Second,
})
```

Tool and rerank checks require explicit opt-in and the corresponding advertised capability. Tool checks force a named function and validate its call ID, name, and JSON arguments. Rerank checks require nonempty results, unique in-range indexes, and finite scores.

The post-start cancellation check waits for a first chunk, cancels the context, and requires channel closure within the configured timeout. An error chunk without closure does not pass. The existing pre-canceled-context check remains separately controlled by `CheckStreamCancellation`. New checks default to disabled, preserving existing harness invocations.

Use local fixture providers for deterministic tests. Invoking the harness against a live provider can make real generation requests.
