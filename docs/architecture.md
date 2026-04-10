# Architecture

## Overview

mcp-banana is a Go MCP (Model Context Protocol) server that wraps Google's Gemini image generation API. It exposes four tools to Claude Code over either a local stdio transport or a remote HTTP transport with bearer token authentication.

The server is organized around a security-first principle: secrets never cross trust boundaries, raw API errors are never forwarded to clients, and all user input is validated before reaching the Gemini API.

## High-Level Architecture

![High-Level Architecture](diagrams/high-level-architecture.png)

## Package Layout

| Path | Responsibility | Key Exports |
|---|---|---|
| `cmd/mcp-banana/` | Entry point; parses flags, wires dependencies, starts transport | `main()` |
| `internal/config/` | Loads and validates environment variables at startup | `Config`, `Load()` |
| `internal/gemini/` | Gemini API client, model registry, error mapping | `Client`, `GeminiService`, `ModelInfo`, `MapError()`, `ValidateRegistryAtStartup()` |
| `internal/security/` | Input validation and output sanitization | `ValidatePrompt()`, `ValidateAndDecodeImage()`, `SanitizeString()`, `RegisterSecret()` |
| `internal/server/` | HTTP routing, middleware chain, health check | `NewMCPServer()`, `NewHTTPHandler()`, `WrapWithMiddleware()` |
| `internal/tools/` | MCP tool handler factories | `NewGenerateImageHandler()`, `NewEditImageHandler()`, `NewListModelsHandler()`, `NewRecommendModelHandler()` |
| `internal/policy/` | Model recommendation logic | `Recommend()`, `Recommendation` |
| `internal/oauth/` | OAuth 2.1 authorization (PKCE, token store, provider delegation, dynamic client registration) | `Handler`, `TokenStore`, `Provider` |

## Package Dependencies

![Package Dependencies](diagrams/package-dependencies.png)

The dependency graph is strictly layered:

- `cmd/` imports `config`, `gemini`, `security`, `server`
- `server` imports `config`, `gemini`, `tools`
- `tools` imports `gemini`, `security`, `policy`
- `security` imports `gemini` (for the alias allowlist)
- `gemini`, `config`, `policy` import only the standard library and third-party SDKs

There are no circular imports. Internal packages are never imported by packages above them in the stack.

## Request Flow

![Request Flow](diagrams/request-flow.png)

### HTTP Transport

A request arriving over HTTP passes through the following stages:

1. **Panic recovery** - a deferred function catches any panics and returns 500 instead of crashing.
2. **Health check bypass** - requests to `/healthz` skip all middleware and return `{"status":"ok"}` immediately.
3. **Bearer token authentication** - checks `Authorization: Bearer <token>` against configured tokens. Missing or invalid tokens return 401.
4. **Rate limiting** - a token bucket limiter enforces `MCP_RATE_LIMIT` requests per minute. Excess requests receive 429 with a `Retry-After` header.
5. **Global concurrency semaphore** - limits simultaneous in-flight requests to `MCP_GLOBAL_CONCURRENCY`. Requests that cannot acquire a slot within 5 seconds receive 503.
6. **Body size enforcement** - request body capped at 15 MB. Oversized bodies receive 413.
7. **MCP protocol dispatch** - the mcp-go library parses the JSON-RPC request and calls the registered tool handler.
8. **Tool handler** - validates inputs, calls the Gemini service, maps errors, and returns a JSON-RPC response.

### Stdio Transport

In stdio mode the middleware chain is not used. Claude Code launches the `mcp-banana` binary directly, and JSON-RPC messages are exchanged over stdin/stdout. Authentication is implicit: only the local user who launched the process can communicate with it.

## Middleware Chain

![Middleware Chain](diagrams/middleware-chain.png)

The middleware chain is defined in `internal/server/middleware.go` and applied via `mw.WrapHTTP()`. `/healthz` bypasses all security middleware. Each layer is a closure that calls the next handler or short-circuits with a JSON error response.

See [security.md](security.md) for the HTTP error contract and threat model details.

## Startup Sequence

![Startup Sequence](diagrams/startup-sequence.png)

The startup sequence in `cmd/mcp-banana/main.go` follows a strict fail-fast order:

1. **Flag parsing** - `--transport`, `--addr`, `--healthcheck`, `--version`
2. **Config loading** - `config.Load()` reads all environment variables and validates them
3. **HTTP transport guard** - if `--transport http` and `MCP_AUTH_TOKEN` is empty, the process exits
4. **Registry validation** - `gemini.ValidateRegistryAtStartup()` checks for sentinel model IDs
5. **Secret registration** - `GEMINI_API_KEY` and `MCP_AUTH_TOKEN` registered with the sanitizer
6. **Logger initialization** - structured JSON logger created with the configured log level
7. **Gemini client creation** - `genai.Client` initialized with the API key and pro-model semaphore
8. **MCP server creation** - four tool handlers registered with the mcp-go server
9. **Transport start** - either `server.ServeStdio()` or an `http.Server` with graceful shutdown

## Security Boundaries

![Security Boundaries](diagrams/security-boundaries.png)

Three distinct trust zones exist:

- **Claude Code** (untrusted input) - sends tool calls over stdio or HTTP; all input is validated before use
- **mcp-banana** (trusted server) - holds secrets in memory, validates input, maps errors
- **Gemini API** (external service) - receives only validated prompts and decoded image bytes; raw responses are never forwarded directly

See [security.md](security.md) for threat model details and security controls.
