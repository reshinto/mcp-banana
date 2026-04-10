## Architecture Rules

### Overview

mcp-banana is a Go MCP server that exposes Gemini image generation to Claude Code via the MCP protocol. It supports dual transport: stdio (for Claude Code direct integration) and HTTP (for standalone deployment on DigitalOcean).

### Package Layout

```
cmd/mcp-banana/       — entry point; selects transport (stdio vs HTTP)
internal/config/      — env loading and config struct validation
internal/gemini/      — Gemini API client + model registry
internal/policy/      — model recommendation logic (prompt → model selection)
internal/security/    — input validation + output sanitization
internal/server/      — MCP server construction, middleware, routing
internal/tools/       — MCP tool handler implementations
```

### Security Boundaries

- The `internal/gemini/` package is the ONLY place that calls the Gemini API. No other package imports genai directly.
- The `internal/security/` package gates ALL input before it reaches `internal/gemini/` and ALL output before it reaches MCP clients.
- `genai.APIError` must never propagate past `internal/gemini/` — wrap with a safe error before returning.
- Secrets (API keys, tokens) must never appear in error messages, logs, or tool responses.

### Model Registry

`internal/gemini/registry.go` is the single source of truth for all model IDs. No model ID string literals appear elsewhere in the codebase.

### Directory Organization

- Group files by domain package, not by file type.
- Co-locate `_test.go` files with the source they test.
- No cross-package deep imports — use the package interface, not internal fields.

### Adding New Features

1. Identify which internal package the feature belongs to.
2. Implement the feature in that package (logic + types + tests in `_test.go`).
3. If the feature introduces a new MCP tool, register it in `internal/tools/` and wire it in `internal/server/`.
4. If the feature uses a new model, add the model ID to `internal/gemini/registry.go` first.
5. Update README in the same pass.

### General Constraints

- One responsibility per function; one objective per struct/type.
- Max 500 lines per file — split files that grow beyond this.
- Prefer composition over embedding for behavioral reuse.
- No business logic in `cmd/` — it wires dependencies only.
- Source files are first-class artifacts — format and lint them like production code.
