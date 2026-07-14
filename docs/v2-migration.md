# Migrating to Wormhole v2

Wormhole v2 keeps the provider bridge focused on app-facing generation while
moving the supported API out of the historical `pkg` tree. Runtime behavior,
wire formats, defaults, exported API names, and CLI behavior are otherwise
unchanged.

## Upgrade the module

```bash
go get github.com/garyblankenship/wormhole/v2@latest
go mod tidy
```

Update imports using this mapping:

| v1 import | v2 import |
| --- | --- |
| `github.com/garyblankenship/wormhole/pkg/wormhole` | `github.com/garyblankenship/wormhole/v2` |
| `github.com/garyblankenship/wormhole/pkg/types` | `github.com/garyblankenship/wormhole/v2/types` |
| `github.com/garyblankenship/wormhole/pkg/config` | `github.com/garyblankenship/wormhole/v2/config` |
| `github.com/garyblankenship/wormhole/pkg/discovery` | `github.com/garyblankenship/wormhole/v2/discovery` |
| `github.com/garyblankenship/wormhole/pkg/discovery/fetchers` | `github.com/garyblankenship/wormhole/v2/discovery/fetchers` |
| `github.com/garyblankenship/wormhole/pkg/middleware` | `github.com/garyblankenship/wormhole/v2/middleware` |
| `github.com/garyblankenship/wormhole/pkg/providers/...` | `github.com/garyblankenship/wormhole/v2/providers/...` |
| `github.com/garyblankenship/wormhole/pkg/testing` | `github.com/garyblankenship/wormhole/v2/wormholetest` |

The CLI install path also includes the major version:

```bash
go install github.com/garyblankenship/wormhole/v2/cmd/wormhole@latest
```

## Removed public packages

`pkg/adapters` has no v2 replacement. Define the interface your application
needs and keep orchestration, pricing, and health policy with the consumer that
owns those decisions. The interface can depend on `types.Provider` without a
Wormhole-owned adapter.

`pkg/validation` and `pkg/providers/transform` were implementation packages.
They are internal in v2 and cannot be imported by consumers. Use the public
schema and provider APIs instead.

Wormhole v2 intentionally provides no forwarding packages or compatibility
shims. Existing v1 applications can remain on the latest v1 release while the
import-path migration is scheduled.
