# Changelog

## v1.14.0 (2026-06-09)

### Features
- add Gemini image generation support with native `generateContent` image responses

### Other
- update package metadata files
- update package tests

## v1.13.0 (2026-06-04)

### Features
- add per-chunk stream idle timeout via `WithStreamIdleTimeout` option and watchdog wrapper
- add stream lifecycle trace events via `StreamTraceFunc` and `WithStreamTrace` (started/ended/error)
- derive proxy route prefixes from `provider_profiles.json` data instead of hardcoded provider lists

### Fixes
- repair tool-call conversations at the `PrepareMessages` boundary — UTF-8 sanitization, ID synthesis, duplicate detection

### Other
- split anthropic transform into focused files (messages, response, stream, tools) and extract gemini system-message handling; broaden test coverage

## v1.10.0 (2026-05-29)

### Features
- add context-aware model discovery methods and guard background refresh startup
- harden proxy server request handling with capped bodies, context propagation, and flexible embedding input forms

### Fixes
- preserve Anthropic system messages sent through normal message lists
- merge partial discovery config with defaults and add explicit discovery opt-outs
- make proxy model listing use configured and discoverable providers
- make load-balancer health-check shutdown and test mock providers race-safe
- restore local lint, example, and release verification targets
- update CLI version reporting to use build metadata

### Other
- refresh GoReleaser config for current syntax and release paths
- tune lint policy and clean existing lint findings
- format existing Go files touched by verification

## v1.9.2 (2026-05-16)

### Other
- trim non-library files from generated source archives

## v1.9.1 (2026-05-16)

### Fixes
- remove local `.work` scratch files from the published module

## v1.9.0 (2026-05-16)

### Features
- add OpenAI-compatible proxy server with model prefix routing
- add agentic loop primitive with scoped tool registry

### Fixes
- wire OpenAI image generation capability through the public image builder
- harden orchestration usage handling

### Other
- refresh project README and banner
- prune unused indirect modules
- expand deterministic test coverage
- align provider and guide examples
- document proxy server usage, installation, and model prefix routing
- document agent loop with usage examples
- update model names in package docs to current versions
- fix API signatures, import paths, broken links, and remove doc test files
- move scratch files to `.work` and normalize doc filenames to lowercase
- update copyright year and Makefile comment
