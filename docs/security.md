# Security

## Overview

mcp-banana implements a defense-in-depth security model. Secrets are isolated in server memory and never cross any trust boundary. All user input is validated before reaching the Gemini API. Raw API errors are mapped to a strict allowlist of safe messages before being returned to Claude Code.

See [architecture.md](architecture.md) for security boundary diagrams and the middleware chain.

## Threat Model

### Trust Boundaries

| Boundary | Authentication |
|---|---|
| Claude Code to mcp-banana (stdio) | Implicit: local process ownership |
| Claude Code to mcp-banana (HTTP, static token) | Bearer token in `Authorization` header |
| Claude Desktop to mcp-banana (HTTP, OAuth) | OAuth 2.1 access token in `Authorization` header |
| mcp-banana to Gemini API | Per-user Gemini API key from credentials file |
| mcp-banana to logs / responses | Sanitizer redacts secrets before any output |

### What Is Protected

- **Per-user Gemini API keys** — registered with the sanitizer on extraction; never logged
- **Internal Gemini model IDs** — only the public Nano Banana aliases are exposed to clients
- **Raw Gemini API errors** — translated to one of five safe error codes; raw SDK text is discarded
- **OAuth provider client secrets** — registered with the sanitizer at startup; redacted from all output

### What Is Not Protected

- **Prompt content** — prompts are forwarded to the Gemini API as-is. Do not include secrets in prompts.
- **Generated image content** — the server does not inspect or filter generated image data beyond MIME type validation.
- **Claude Code's identity** — in stdio mode, the server trusts the local process. In HTTP mode, authentication is token-based, not identity-based.

### Security Controls at Each Boundary

| Control | Where Applied | Configurable |
|---|---|---|
| Bearer token auth | HTTP middleware, every request except `/healthz` | `MCP_CREDENTIALS_FILE` |
| OAuth access token auth | HTTP middleware fallback (after static token check) | `OAUTH_*` env vars + `OAUTH_BASE_URL` |
| Rate limiting | HTTP middleware | `MCP_RATE_LIMIT` |
| Global concurrency limit | HTTP middleware | `MCP_GLOBAL_CONCURRENCY` |
| Pro-model concurrency limit | Gemini client, per-client semaphore | `MCP_PRO_CONCURRENCY` |
| Body size limit | HTTP middleware | Fixed at 15 MB |
| Request timeout | Gemini client, context deadline | `MCP_REQUEST_TIMEOUT_SECS` |
| Input validation | Tool handlers, security package | Fixed constraints (see below) |
| Secret sanitization | Startup registration + all log/response paths | Automatic |
| Error mapping | Gemini client and tool handlers | Fixed allowlist |
| Panic recovery | HTTP middleware | Always active |
| Registry sentinel check | Startup, `ValidateRegistryAtStartup()` | Blocks boot if triggered |
| PKCE enforcement | OAuth handler, every authorization request | Fixed; rejects requests without `code_challenge_method=S256` |
| OAuth state parameter | OAuth handler, per-request CSRF token | Generated via `crypto/rand`, verified on callback |
| Redirect URI validation | OAuth handler, callback stage | Fixed allowlist per registered client |
| OAuth token TTL | Token store | Auth code: 10 min; access token: 1 hr; refresh token: 30 days |
| Provider secret sanitization | Startup secret registration | All `OAUTH_*_CLIENT_SECRET` values registered with sanitizer |
| TLS | HTTP server (optional) | `MCP_TLS_CERT_FILE` + `MCP_TLS_KEY_FILE` |

## Secret Management

At startup, the following values are registered with the sanitizer:

```go
security.RegisterSecret(serverConfig.OAuthGoogleClientSecret)
security.RegisterSecret(serverConfig.OAuthGitHubClientSecret)
security.RegisterSecret(serverConfig.OAuthAppleClientSecret)
```

Per-request Gemini API keys are registered as they arrive:

```go
geminiAPIKey := request.Header.Get("X-Gemini-API-Key")
if geminiAPIKey != "" {
    security.RegisterSecret(geminiAPIKey)
    // ...
}
```

`SanitizeString` applies three passes to any string before it is logged or returned:

1. Replace every registered secret value with `[REDACTED]`.
2. Replace any pattern matching `AIza[0-9A-Za-z_-]{35}` (the Gemini API key format) with `[REDACTED]`.
3. Strip `\n` and `\r` characters to prevent log injection.

The `sync.RWMutex` in `sanitize.go` makes these operations safe for concurrent use. Registration uses a write lock; sanitization takes a read lock and copies the slice before iterating.

`ClearSecrets()` is available for use in tests only, to prevent state leakage between test functions.

## Input Validation

All user-supplied input is validated in `internal/security/validate.go` before being forwarded to the Gemini client. Every tool handler calls the appropriate validators and returns an error to the MCP client if any check fails.

| Validator | Input | Constraints |
|---|---|---|
| `ValidatePrompt` | `prompt` string | Non-empty; max 10,000 runes; no null bytes (`\x00`) |
| `ValidateModelAlias` | model alias string | Empty accepted (caller uses default `nano-banana-2`); non-empty value must be in the registry allowlist |
| `ValidateAspectRatio` | aspect ratio string | Empty accepted; non-empty value must be one of: `1:1`, `16:9`, `9:16`, `4:3`, `3:4` |
| `ValidatePriority` | priority string | Empty accepted; non-empty value must be one of: `speed`, `quality`, `balanced` |
| `ValidateTaskDescription` | task description string | Non-empty; max 1,000 runes |
| `ValidateAndDecodeImage` | base64 image + MIME type | Non-empty; valid base64; decoded size within `MCP_MAX_IMAGE_BYTES`; MIME type must be `image/png`, `image/jpeg`, or `image/webp`; minimum 12 decoded bytes for magic byte validation; magic bytes must match the declared MIME type |

Magic byte validation checks the raw binary header of the decoded image to prevent a client from declaring a false MIME type:

| MIME type | Check |
|---|---|
| `image/png` | First 4 bytes equal `\x89PNG` |
| `image/jpeg` | First 3 bytes equal `0xFF 0xD8 0xFF` |
| `image/webp` | Bytes 0–3 equal `RIFF` and bytes 8–11 equal `WEBP` |

## Error Mapping Boundary

`internal/gemini/errors.go` is the critical security boundary between raw Gemini SDK errors and Claude Code. The raw `genai.APIError` type can contain request metadata, headers, and API keys in its message text. None of that text is ever forwarded.

The mapping works in two stages:

1. If the error wraps a `*genai.APIError`, extract only the HTTP status code and map it to a safe code using `mapHTTPStatus`.
2. Otherwise, classify by substring patterns in the error message text (for classification only — the matched text is discarded and never returned).

Five safe error codes form the complete allowlist. No other error text may reach Claude Code:

| Code | Meaning | Returned when |
|---|---|---|
| `content_policy_violation` | Prompt blocked by Gemini content safety | HTTP 400/403 from API; "safety" or "blocked" in error text |
| `quota_exceeded` | API quota or rate limit exceeded | HTTP 429 from API; "quota" or "rate" in error text |
| `model_unavailable` | Requested model not found or deprecated | HTTP 404 from API; "not found" or "deprecated" in error text |
| `generation_failed` | Image generation failed; retry is safe | HTTP 5xx from API; catch-all for unclassified errors |
| `server_error` | Internal server error | Panic recovery; context cancellation waiting for pro slot |

## HTTP Error Contract

| Situation | HTTP Status | JSON Body |
|---|---|---|
| Missing or invalid bearer / OAuth token | 401 | `{"error":"unauthorized"}` |
| Rate limit exceeded | 429 | `{"error":"rate_limited"}` (+ `Retry-After` header) |
| Request body exceeds 15 MB | 413 | `{"error":"request_too_large"}` |
| Request body read error | 400 | `{"error":"bad_request"}` |
| Server queue timeout (all slots busy) | 503 | `{"error":"server_busy"}` |
| Unexpected server panic | 500 | `{"error":"server_error"}` |
| Health check | 200 | `{"status":"ok"}` |

## Per-User API Key Security

The `X-Gemini-API-Key` header allows individual users to supply their own Gemini API key. This path has specific security properties:

1. The key is extracted in the middleware before any rate limiting or tool dispatch.
2. `security.RegisterSecret` is called immediately so the value is redacted in all subsequent log output.
3. The key is stored in the request context using a package-private context key (`gemini.contextKey`), preventing accidental access by unrelated packages.
4. The `ClientCache` caches clients by API key under a `sync.RWMutex` with double-checked locking to prevent duplicate client creation under concurrent requests.
5. The key is never echoed back to the client, included in error messages, or stored outside of the in-process cache.

## OAuth Security

### PKCE Requirement

Every OAuth authorization request must include `code_challenge` and `code_challenge_method=S256`. The `/authorize` handler rejects requests missing either parameter before any provider redirect occurs. At the `/token` endpoint, `VerifyCodeChallenge` recomputes `SHA-256(code_verifier)` encoded as base64url and compares it against the stored challenge. Only `S256` is accepted; plain method is not supported.

### State Parameter (CSRF Protection)

For each upstream provider redirect, the server generates a 16-byte cryptographically random state token via `crypto/rand`. The token is stored in the `oauth.Store` as a `ProviderSession`. The callback handler looks up the session by the returned state value and rejects requests with a missing or unknown state before any code exchange occurs. Sessions are single-use (consumed on read) and expire after 10 minutes.

### Redirect URI Validation

The `/authorize` handler validates the requested `redirect_uri` against the allowlist stored for the client in `oauth.Store` at registration time. Any callback request whose redirect URI does not match exactly is rejected with `400 invalid_redirect_uri` before the authorization flow proceeds.

### Token TTLs

| Token type | Lifetime | Semantics |
|---|---|---|
| Authorization code | 10 minutes | Single-use; consumed and deleted immediately on exchange |
| Provider session (state) | 10 minutes | Single-use; consumed and deleted on callback |
| Access token | 1 hour | Multi-use; validated by `Store.ValidateAccessToken` on each request |
| Refresh token | 30 days | Single-use; consumed and replaced on each refresh grant |

Short-lived access tokens limit the exposure window if a token is intercepted. Refresh tokens are rotated on every use.

### Provider Secret Sanitization

All `OAUTH_*_CLIENT_SECRET` values are registered with `security.RegisterSecret()` at startup. This ensures they are redacted from logs and error messages even if they appear in unexpected code paths.

### TLS Requirements

OAuth 2.1 requires TLS for all token-bearing requests in production. Enable TLS on the mcp-banana HTTP server by setting both `MCP_TLS_CERT_FILE` and `MCP_TLS_KEY_FILE`. Without TLS, access tokens and authorization codes are transmitted in plaintext. The health check (`--healthcheck` flag) automatically switches to HTTPS when `MCP_TLS_CERT_FILE` is set.

### Dynamic Client Registration

The `/register` endpoint implements RFC 7591. It accepts a `client_name` and `redirect_uris`, generates a random 16-byte `client_id`, stores the client in the in-memory `oauth.Store`, and returns `201 Created` with the registration details. No client secret is issued (public client model). Registered clients are held in memory only; they do not persist across server restarts.
