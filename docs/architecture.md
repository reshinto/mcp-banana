# Architecture

## Overview

mcp-banana is a Go MCP (Model Context Protocol) server that wraps Google's Gemini image generation API. It exposes four tools to Claude Code and Claude Desktop over either a local stdio transport or a remote HTTP transport with bearer token or OAuth 2.1 authentication.

The server is organized around a security-first principle: secrets never cross trust boundaries, raw API errors are never forwarded to clients, and all user input is validated before reaching the Gemini API.

## Package Layout

| Path | Responsibility | Key Types / Exports |
|---|---|---|
| `cmd/mcp-banana/` | Entry point; parses flags, wires dependencies, starts transport | `main()`, `run()` |
| `internal/config/` | Loads and validates all environment variables at startup | `Config`, `Load()` |
| `internal/gemini/` | Gemini API client, per-user client cache, model registry, error mapping | `Client`, `ClientCache`, `GeminiService`, `ModelInfo`, `SafeModelInfo`, `MapError()`, `ValidateRegistryAtStartup()` |
| `internal/security/` | Input validation and output sanitization | `ValidatePrompt()`, `ValidateModelAlias()`, `ValidateAspectRatio()`, `ValidatePriority()`, `ValidateTaskDescription()`, `ValidateAndDecodeImage()`, `SanitizeString()`, `RegisterSecret()` |
| `internal/server/` | HTTP routing, middleware chain, health check, MCP server construction | `NewMCPServer()`, `NewHTTPHandler()`, `WrapWithMiddleware()` |
| `internal/tools/` | MCP tool handler factories | `NewGenerateImageHandler()`, `NewEditImageHandler()`, `NewListModelsHandler()`, `NewRecommendModelHandler()` |
| `internal/policy/` | Model recommendation logic (prompt characteristics → model selection) | `Recommend()`, `Recommendation` |
| `internal/oauth/` | OAuth 2.1 authorization: PKCE, token store, provider delegation, dynamic client registration, server metadata | `Handler`, `Store`, `ProviderConfig`, `NewAuthorizeHandler()`, `NewCallbackHandler()`, `NewTokenHandler()`, `NewMetadataHandler()`, `NewRegistrationHandler()` |

## Package Dependencies

The dependency graph is strictly layered:

- `cmd/` imports `config`, `gemini`, `security`, `server`, `oauth`
- `server` imports `config`, `gemini`, `tools`, `oauth`
- `tools` imports `gemini`, `security`, `policy`
- `security` imports `gemini` (for the alias allowlist)
- `gemini`, `config`, `policy` import only the standard library and third-party SDKs
- `oauth` imports only the standard library

There are no circular imports. Internal packages are never imported by packages above them in the stack.

## Dual Transport

mcp-banana supports two transports selected at startup via the `--transport` flag.

### Stdio Transport (Claude Code direct integration)

The binary is launched directly by Claude Code. JSON-RPC messages are exchanged over stdin/stdout using the mcp-go `ServeStdio` function. The HTTP middleware chain is not used. Authentication is implicit: only the local user who launched the process can communicate with it. This is the default transport.

### HTTP Transport (standalone / Docker / DigitalOcean)

The binary binds to a TCP address (default `0.0.0.0:8847`) and serves the MCP protocol over HTTP at `/mcp` using the mcp-go streamable HTTP transport. All requests pass through the middleware chain. Optional TLS is available via `MCP_TLS_CERT_FILE` and `MCP_TLS_KEY_FILE`. Graceful shutdown waits up to 120 seconds for in-flight requests to complete.

## Request Flow

```
Claude Code / Claude Desktop
        │
        ├─ stdio ──────────────────────────────────────────┐
        │                                                   │
        └─ HTTP ──► middleware chain ─────────────────────►│
                    (panic recovery                         │
                     healthz bypass                         │
                     bearer/OAuth auth                      │
                     X-Gemini-API-Key extraction            │
                     rate limiting                          │
                     concurrency semaphore                  │
                     body size limit)                       │
                                                            ▼
                                                    MCP server (mcp-go)
                                                            │
                                                            ▼
                                                    Tool handler
                                                    (internal/tools/)
                                                            │
                                                    Security validation
                                                    (internal/security/)
                                                            │
                                                    Resolve Gemini client
                                                    (ClientCache.GetClient)
                                                            │
                                                            ▼
                                                    Gemini client
                                                    (internal/gemini/)
                                                            │
                                                            ▼
                                                    Gemini API (Google)
                                                            │
                                                            ▼
                                                    Response sanitization
                                                    (error mapping, alias
                                                     substitution, MIME check)
                                                            │
                                                            ▼
                                                       MCP response
                                                    → Claude Code / Desktop
```

## Middleware Chain

The chain is defined in `internal/server/middleware.go` and applied to all HTTP traffic via `mw.WrapHTTP()`. Layers execute in this exact order:

1. **Panic recovery** — a deferred function catches any unhandled panics and returns `500 {"error":"server_error"}` instead of crashing the process.
2. **Health check bypass** — requests to `/healthz` skip all remaining middleware and return `{"status":"ok"}` immediately.
3. **Bearer / OAuth auth** — checks `Authorization: Bearer <token>` in order: (a) credentials file lookup (`MCP_CREDENTIALS_FILE`, maps tokens to Gemini API keys), (b) OAuth access token via `Store.ValidateAccessToken` (resolves provider identity to Gemini key in credentials file), (c) self-registration (unknown token + valid `X-Gemini-API-Key` header registers a new entry). If no auth is configured at all, all requests pass through (SSH tunnel expected for network-level security).
4. **X-Gemini-API-Key extraction** — if the `X-Gemini-API-Key` header is present, the value is registered with `security.RegisterSecret` (to ensure it is redacted from logs) and stored in the request context via `gemini.WithAPIKey`. This enables per-user Gemini client resolution downstream.
5. **Rate limiting** — a token bucket limiter enforces `MCP_RATE_LIMIT` requests per minute. Excess requests receive `429` with a `Retry-After` header.
6. **Global concurrency semaphore** — limits simultaneous in-flight requests to `MCP_GLOBAL_CONCURRENCY`. Requests that cannot acquire a slot within 5 seconds receive `503 {"error":"server_busy"}`.
7. **Body size enforcement** — request body is capped at 15 MB. Oversized bodies receive `413 {"error":"request_too_large"}`. The body is pre-read into memory so the size check happens before any further processing.

## OAuth Flow

OAuth 2.1 is optional and activated when at least one provider credential pair (`OAUTH_*_CLIENT_ID` + `OAUTH_*_CLIENT_SECRET`) is configured along with `OAUTH_BASE_URL`. When active, the following HTTP routes are registered:

| Route | Handler | Purpose |
|---|---|---|
| `/.well-known/oauth-authorization-server` | `NewMetadataHandler` | RFC 8414 server discovery (used by Claude Desktop) |
| `/register` | `NewRegistrationHandler` | RFC 7591 dynamic client registration |
| `/authorize` | `NewAuthorizeHandler` | Initiates authorization; renders provider login page |
| `/callback` | `NewCallbackHandler` | Handles provider redirect; issues MCP authorization code |
| `/token` | `NewTokenHandler` | Token exchange and refresh (authorization_code, refresh_token) |

Supported upstream providers: Google, GitHub, Apple. See [security.md](security.md) for PKCE, state, and token TTL details.

## Per-User API Key Architecture

The server supports per-request Gemini API keys to allow multi-user deployments where each user brings their own key.

1. The HTTP client includes `X-Gemini-API-Key: <key>` in the request header.
2. The middleware extracts the key, registers it as a secret with the sanitizer, and stores it in the request context.
3. The tool handler calls `gemini.APIKeyFromContext(ctx)` to retrieve the key.
4. The handler calls `clientCache.GetClient(ctx, apiKey)`, which returns a cached per-user `*Client` or creates and caches a new one.
5. If no per-request key is available, the request is rejected with an error.

All per-user clients share the same `timeoutSecs` and `proConcurrency` settings from server config.

## Model Registry

`internal/gemini/registry.go` is the single source of truth for all model identifiers. The registry maps public Nano Banana aliases (e.g. `nano-banana-2`) to internal Gemini model IDs (e.g. `gemini-3.1-flash-image-preview`).

Two types enforce the alias/ID separation:

- `ModelInfo` — includes `GeminiID`. Used only inside the `gemini` package for API calls.
- `SafeModelInfo` — deliberately excludes `GeminiID`. Used for all data returned to Claude Code via `AllModelsSafe()`.

No `GeminiID` value ever appears in a tool response, log entry, or error message. The sentinel value `VERIFY_MODEL_ID_BEFORE_RELEASE` causes `ValidateRegistryAtStartup()` to fail at boot, preventing deployment with unverified model IDs.

## Security Boundaries

Three trust zones exist:

| Zone | Trust Level | Entry / Exit Point |
|---|---|---|
| Claude Code / Claude Desktop | Untrusted input | Middleware auth check; input validated by `internal/security/` before use |
| mcp-banana process | Trusted | Holds secrets in memory; validates input; maps errors to safe codes |
| Gemini API | External service | Receives only validated prompts and decoded image bytes; raw responses are filtered before forwarding |

Key boundary enforcements:

- **Input validation** — all user-supplied fields pass through `internal/security/validate.go` before reaching `internal/gemini/`.
- **Secret sanitization** — `security.SanitizeString` is applied to all strings that appear in logs or error output. Secrets and API key patterns are redacted to `[REDACTED]`.
- **Error mapping** — `gemini.MapError()` translates raw `genai.APIError` values to one of five safe error codes. Raw SDK error text is never forwarded.
- **Model ID isolation** — tool handlers receive `SafeModelInfo` (no `GeminiID`). The `GeminiID` is resolved inside `internal/gemini/` just before the API call and never leaves that package.

## Startup Sequence

The startup sequence in `cmd/mcp-banana/main.go` follows a strict fail-fast order:

1. **Flag parsing** — `--transport`, `--addr`, `--healthcheck`, `--version`
2. **Config loading** — `config.Load()` reads and validates all environment variables; exits on any missing or malformed value
3. **Registry validation** — `gemini.ValidateRegistryAtStartup()` checks for sentinel model IDs; exits if any are found
4. **Secret registration** — all `OAUTH_*_CLIENT_SECRET` values are registered with `security.RegisterSecret`
5. **Logger initialization** — structured JSON logger created with the configured log level (output to stderr)
6. **Gemini client creation** — default `*gemini.Client` and `*gemini.ClientCache` initialized with the API key, timeout, and pro-model semaphore
7. **OAuth setup** — active providers assembled from config; if any are present, an `oauth.Store` is created and a background cleanup goroutine is started
8. **MCP server creation** — four tool handlers registered with the mcp-go server
9. **Transport start** — either `server.ServeStdio()` (stdio mode) or an `http.Server` with graceful shutdown (HTTP mode)
