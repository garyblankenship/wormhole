# Run Codex with Z.AI through Wormhole

Use Wormhole 1.24.0 or newer to give Codex's Responses API client a local
bridge to the Z.AI GLM Coding Plan API.

```bash
go install github.com/garyblankenship/wormhole/v2/cmd/wormhole@latest
export ZAI_API_KEY="your-zai-api-key"
export WORMHOLE_API_KEY="YOUR_API_KEY"
wormhole serve --default-provider zai
```

Leave that process running. Wormhole listens on `127.0.0.1:8080` by default
and sends unprefixed models such as `glm-5.2` to Z.AI.

> [!IMPORTANT]
> Wormhole provides the technical OpenAI-compatible bridge, but Z.AI currently
> limits Coding Plan subscription benefits to its
> [officially supported tools](https://docs.z.ai/devpack/tool/others). Codex and
> Wormhole are not named on that list. Confirm eligibility with Z.AI before
> using a Coding Plan key through this setup; unsupported use may restrict plan
> benefits. For other use, use a normal metered Z.AI API key and set
> `ZAI_BASE_URL=https://api.z.ai/api/paas/v4`.

## Add an alternative Codex profile

Copy the ready-to-use [`zai.config.toml`](../examples/codex/zai.config.toml), or
save the following as `~/.codex/zai.config.toml` (or
`$CODEX_HOME/zai.config.toml` when `CODEX_HOME` is set):

```toml
model_provider = "zai"
model = "glm-5.2"
model_context_window = 1000000
model_supports_reasoning_summaries = false
model_catalog_json = "zai-models.json"

[model_providers.zai]
name = "Z.AI via Wormhole"
base_url = "http://127.0.0.1:8080/v1"
wire_api = "responses"
env_key = "WORMHOLE_API_KEY"
```

Download [`zai-models.json`](../examples/codex/zai-models.json) beside the
profile:

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}"
curl -fsSL \
  https://raw.githubusercontent.com/garyblankenship/wormhole/main/docs/examples/codex/zai-models.json \
  -o "${CODEX_HOME:-$HOME/.codex}/zai-models.json"
```

The catalog supplies Codex with GLM-5.2's context and tool metadata. Without
it, Codex can still start, but it warns that model metadata was not found and
uses fallback limits. The relative catalog path resolves from the directory
containing the selected profile.

This is an alternative profile: it does not replace your default
`config.toml`. Codex profile files live beside `config.toml` and follow the
`<name>.config.toml` naming convention.

## Start Codex

Open an interactive coding session:

```bash
export WORMHOLE_API_KEY="YOUR_API_KEY"
codex -p zai
```

Run a non-interactive smoke test:

```bash
codex exec -p zai --ephemeral --skip-git-repo-check -s read-only \
  'Reply exactly ZAI_OK and do not use tools.'
```

Run Codex against a specific project:

```bash
codex -p zai -C /path/to/project
```

The `-p zai` flag layers `zai.config.toml` over your normal user configuration,
so your instructions, rules, skills, and other settings continue to apply
unless the profile overrides them.

## What Wormhole translates

Codex sends `POST /v1/responses` requests to the local proxy. Wormhole routes
them to Z.AI's OpenAI-compatible Chat Completions endpoint and translates the
response back to the Responses API event stream Codex expects.

The bridge covers the coding-agent path:

- streaming Responses API lifecycle events;
- text input;
- function tools and Codex custom tools such as `apply_patch`;
- tool-call output continuation across turns;
- `allowed_tools` selection; and
- Codex-compatible `GET /v1/models` responses.

Z.AI reasoning effort is translated to its `thinking` provider option. The
bridge currently treats `low`, `medium`, and `high` alike: each enables
thinking, while `none` disables it. Native Responses tools without a portable
Chat Completions equivalent, including `web_search` and `namespace`, are not
forwarded. Codex's local shell and patch tools continue to work because they
use the portable custom/function tool path.

## Z.AI endpoint selection

Wormhole's built-in `zai` profile defaults to the GLM Coding Plan endpoint:

```text
https://api.z.ai/api/coding/paas/v4
```

This matters: the general Z.AI endpoint and common guesses such as
`https://api.z.ai/v1` do not target the Coding Plan API and can produce 404s or
billing-plan errors. If Z.AI assigns your account a different regional route,
override the upstream URL when starting Wormhole:

```bash
export ZAI_BASE_URL="https://your-account-specific-coding-endpoint"
wormhole serve --default-provider zai
```

Use the exact Coding Plan endpoint supplied for your account. Keep Codex's
`base_url` pointed at Wormhole (`http://127.0.0.1:8080/v1`), not at Z.AI.

For mainland China Coding Plan accounts, Z.AI currently documents:

```bash
export ZAI_BASE_URL="https://open.bigmodel.cn/api/coding/paas/v4"
```

## Configuration reference

| Setting | Type | Default | Purpose |
| --- | --- | --- | --- |
| Wormhole `--addr` | string | `127.0.0.1:8080` | Local proxy listen address |
| Wormhole `--default-provider` | string | empty | Provider for models without a `provider/` prefix |
| `ZAI_API_KEY` | secret string | unset | Authenticates Wormhole to the upstream Z.AI Coding Plan API |
| `ZAI_BASE_URL` | URL string | Z.AI Coding Plan endpoint | Overrides Wormhole's upstream Z.AI URL |
| `WORMHOLE_API_KEY` | secret string | unset | Authenticates Codex to Wormhole's `/v1/` endpoints |
| Codex `model_provider` | string | `openai` | Selects the `model_providers.zai` table |
| Codex `model` | string | provider-dependent | Selects `glm-5.2` |
| Codex `model_context_window` | integer | model metadata | Declares GLM-5.2's 1,000,000-token context window |
| Codex `model_catalog_json` | file path | unset | Loads the supplied GLM-5.2 metadata catalog |
| Codex provider `base_url` | URL string | unset for custom providers | Sends Codex traffic to Wormhole's `/v1` API |
| Codex provider `wire_api` | `responses` | `responses` | Uses the only custom-provider wire protocol Codex supports |
| Codex provider `env_key` | environment variable name | unset | Reads the Wormhole client token without storing it in TOML |

Wormhole owns the Z.AI upstream URL and translation behavior. Codex owns the
local profile, model metadata, agent loop, and local tools.

## Troubleshooting

### `Model metadata for glm-5.2 not found`

Confirm `zai-models.json` is beside `zai.config.toml` and that the profile has:

```toml
model_catalog_json = "zai-models.json"
```

Then start a new `codex -p zai` session. Existing sessions do not reload model
metadata.

### `404 Not Found` followed by reconnects

Check each hop separately:

```bash
wormhole version
curl -fsS http://127.0.0.1:8080/health
curl -fsS -H "Authorization: Bearer $WORMHOLE_API_KEY" \
  http://127.0.0.1:8080/v1/models
```

Use Wormhole 1.24.0 or newer. Confirm Codex uses
`http://127.0.0.1:8080/v1`, Wormhole starts with
`--default-provider zai`, and `ZAI_BASE_URL` is either unset or points to the
dedicated Coding Plan endpoint. Wormhole logs upstream provider failures
without returning raw upstream URLs or response bodies to Codex.

### `Connection refused`

Start `wormhole serve --default-provider zai` in another terminal and leave it
running. If port 8080 is already in use, choose another loopback port in both
the Wormhole `--addr` flag and the Codex profile's `base_url`.

### Missing API key

Export `ZAI_API_KEY` where Wormhole can read it. Export `WORMHOLE_API_KEY` in
the environments inherited by both Wormhole and Codex. Keep the two values
different, and do not put either value directly in the profile or commit it.

## Keep the proxy local

The setup above intentionally binds only to loopback and still authenticates
the `/v1/` endpoints. If you bind Wormhole to a non-loopback interface,
Wormhole refuses to start unless `WORMHOLE_API_KEY` is set. Put TLS in front of
any network-exposed deployment; the bearer token otherwise travels over plain
HTTP.

See the [Codex configuration reference](https://developers.openai.com/codex/config-reference)
for current profile and custom-provider settings, and the
[Z.AI Coding Plan quick start](https://docs.z.ai/devpack/quick-start) for the
dedicated endpoint and review the
[Coding Plan usage policy](https://docs.z.ai/devpack/usage-policy). Mainland
China accounts should follow the
[regional Coding Plan quick start](https://docs.bigmodel.cn/cn/coding-plan/quick-start).
