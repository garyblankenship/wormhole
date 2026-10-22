# Smoke Checks for OpenAI-Compatible Endpoints

`wormhole.RunOpenAICompatibleSmoke` sends a few real requests through the same
adapter Wormhole uses for normal generation, so it proves the wire path: your
base URL, route, auth and request serialization all reach the server, and the
response parses back into Wormhole's types. It is an endpoint diagnostic, not a
model-quality test.

```go
result, err := wormhole.RunOpenAICompatibleSmoke(ctx, wormhole.OpenAICompatibleSmokeConfig{
	BaseURL: "http://127.0.0.1:8000/v1",
	Model:   "default",
})
```

Wormhole appends `/chat/completions` to the base URL, so local servers usually
need `http://host:port/v1`, not just `http://host:port`.

## Transport success versus semantic success

The smoke separates two questions into distinct checks:

| Check | Question answered | Failure means |
|---|---|---|
| `chat` (transport) | Did the request serialize, reach the server, and parse back into a nonempty response? | Wire/protocol problem: wrong URL, auth, incompatible server |
| `response_validation` (optional, semantic) | Does the parsed response satisfy **your** domain rules? | Your validator's error, not a transport fault |
| `stream` (optional) | Does streaming parse through the same adapter? | Streaming-specific wire problem |
| `embeddings` (optional) | Do embeddings parse? | Embeddings-specific wire problem |

A passing `chat` check is transport evidence only. It says nothing about
whether the model answered in the shape your application needs. Install
`ValidateResponse` when you need that second, semantic answer:

```go
cfg := wormhole.OpenAICompatibleSmokeConfig{
	BaseURL: "http://127.0.0.1:8000/v1",
	Model:   "default",
	ValidateResponse: func(resp *types.TextResponse) error {
		var payload struct {
			Answer string `json:"answer"`
		}
		if err := json.Unmarshal([]byte(resp.Text), &payload); err != nil {
			return fmt.Errorf("response is not the expected JSON object: %w", err)
		}
		if payload.Answer == "" {
			return errors.New("response JSON is missing the answer field")
		}
		return nil
	},
}
result, err := wormhole.RunOpenAICompatibleSmoke(ctx, cfg)
```

Contract of the validator:

- It runs once, after the `chat` check passes, on the parsed
  `*types.TextResponse`. There is no default validator; `nil` keeps the
  historical behavior (no `response_validation` check at all).
- Its error is wrapped, so `errors.Is(err, mySentinel)` works, and it is
  recorded on the failed `response_validation` check.
- On failure the optional `stream` and `embeddings` probes are skipped, and no
  extra requests are made. The chat diagnostics on the result stay populated.
- There are no retries and no implicit limits. Each transport probe is one
  request; the separate semantic check invokes the callback, not another
  endpoint request. With validation and both optional probes enabled, a
  successful run records four checks from three transport requests.
- The SDK never installs one application's shape policy globally — validation
  lives with the caller that owns the contract.

## Model identity diagnostics

The result reports both sides of the model name:

```go
result.Model           // requested model (compatibility field, unchanged)
result.RequestedModel  // requested model
result.ReportedModel   // model identity parsed from the chat response body
```

`ReportedModel` is diagnostic only. An upstream serving an alias (for example
you ask for `default` and the server answers `glm-5.2`) is recorded, not
failed — identity mismatch is evidence about routing, never an automatic
error.

**Known limitation, stated plainly:** when the upstream response body omits
the `model` field entirely, the OpenAI-compatible provider backfills the
requested model, so `ReportedModel` equals `RequestedModel`. The smoke cannot
distinguish that backfill from an upstream echo, and it does not claim
otherwise. A matching pair means "no mismatch was observed", not "the server
echoed the model".

## Finish reason, usage and thinking

When the upstream reports them, the result carries the parsed values:

```go
result.FinishReason // types.FinishReasonStop, FinishReasonLength, ...
result.Usage        // *types.Usage with prompt/completion/total tokens
result.Thinking     // *types.Thinking when reasoning_content was present
```

These are SDK-parsed diagnostics, not independent upstream-presence evidence.
The OpenAI-compatible adapter supplies a nonnil, zero-valued `Usage` when
upstream accounting is omitted; it cannot distinguish omission from reported
zero usage. `Thinking` is nil when absent. Omission is not a smoke error.

## Local fixture versus live model evidence

The examples above prove the transport against a **local fixture** (an
`httptest` server or your local proxy). That is the evidence this smoke is
designed to produce without spending provider credits. Claims about a
**specific live model's** behavior — aliasing, thinking output, usage
accounting, JSON discipline — require running the same smoke against that
live endpoint and reading `ReportedModel`, `FinishReason`, `Usage` and
`Thinking` from *that* run; do not generalize fixture results to live models
or vice versa.
