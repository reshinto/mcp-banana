# OAuth Support for Claude Desktop Integration

**Date:** 2026-04-11
**Status:** Approved

## Problem

Claude Desktop GUI requires MCP servers to implement the MCP OAuth spec (based on OAuth 2.1) for custom connector integration. mcp-banana currently only supports static bearer token authentication, which Claude Desktop does not accept.

## Goal

Add OAuth 2.1 support to mcp-banana so users can connect via Claude Desktop GUI at claude.ai/customize/connectors. Existing bearer token auth remains supported for Claude Code CLI usage.

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Identity providers | Google, GitHub, Apple | Broad coverage for developer and consumer audiences |
| Access control | Open | Any authenticated user can use the server |
| Token storage | In-memory | Simplicity; tokens reset on restart is acceptable |
| TLS | Built-in with manual cert files | Droplet ports 80/443 are in use by personal website |
| Domain | Subdomain (e.g., `banana.yourdomain.com`) | Required for valid TLS cert |
| OAuth endpoints | Built into mcp-banana | Single binary, no extra services |

## Architecture

### New Package: `internal/oauth/`

All OAuth logic lives in a new `internal/oauth/` package with these responsibilities:

- OAuth provider configuration (Google, GitHub, Apple)
- Authorization code generation and validation
- PKCE challenge verification
- Token issuance (access + refresh)
- Dynamic client registration
- In-memory store for codes, tokens, clients, and provider sessions

### Package Layout

```
internal/oauth/
  provider.go       — provider interface + Google/GitHub/Apple configs
  store.go          — in-memory store for codes, tokens, clients
  handler.go        — HTTP handlers for OAuth endpoints
  metadata.go       — authorization server metadata response
  registration.go   — dynamic client registration (RFC 7591)
  pkce.go           — PKCE code challenge/verifier logic
```

### Endpoint Summary

| Endpoint | Method | Purpose |
|---|---|---|
| `/.well-known/oauth-authorization-server` | GET | Authorization server metadata (RFC 8414) |
| `/register` | POST | Dynamic client registration (RFC 7591) |
| `/authorize` | GET | HTML login page with provider buttons |
| `/callback` | GET/POST | OAuth callback from Google/GitHub (GET) and Apple (POST) |
| `/token` | POST | Token exchange (auth code -> access token) and refresh |

### Flow

```
Claude Desktop                mcp-banana                    Google/GitHub/Apple
     |                            |                                |
     |-- POST /mcp -------------->|                                |
     |<-- 401 Unauthorized -------|                                |
     |                            |                                |
     |-- GET /.well-known/... --->|                                |
     |<-- metadata JSON ----------|                                |
     |                            |                                |
     |-- POST /register --------->|                                |
     |<-- client_id, etc ---------|                                |
     |                            |                                |
     |-- GET /authorize?... ----->|                                |
     |<-- HTML login page --------|                                |
     |                            |                                |
     |-- User clicks provider --->|-- redirect to provider ------->|
     |                            |                                |
     |                            |<-- callback with auth code ----|
     |                            |-- exchange code for token ---->|
     |                            |<-- provider access token ------|
     |                            |                                |
     |<-- redirect to client      |                                |
     |   with mcp auth code       |                                |
     |                            |                                |
     |-- POST /token ------------>|                                |
     |   (code + code_verifier)   |                                |
     |<-- access_token + refresh -|                                |
     |                            |                                |
     |-- POST /mcp -------------->|                                |
     |   Authorization: Bearer    |                                |
     |<-- MCP response -----------|                                |
```

### Authorization Server Metadata

Response for `GET /.well-known/oauth-authorization-server`:

```json
{
  "issuer": "https://banana.yourdomain.com:8847",
  "authorization_endpoint": "https://banana.yourdomain.com:8847/authorize",
  "token_endpoint": "https://banana.yourdomain.com:8847/token",
  "registration_endpoint": "https://banana.yourdomain.com:8847/register",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "code_challenge_methods_supported": ["S256"],
  "token_endpoint_auth_methods_supported": ["none"]
}
```

### Dynamic Client Registration

`POST /register` accepts a JSON body per RFC 7591:

```json
{
  "client_name": "Claude Desktop",
  "redirect_uris": ["https://claude.ai/api/mcp/auth_callback"],
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "none"
}
```

Returns:

```json
{
  "client_id": "<generated-uuid>",
  "client_name": "Claude Desktop",
  "redirect_uris": ["https://claude.ai/api/mcp/auth_callback"],
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "none"
}
```

Registered clients are stored in-memory. No client secrets are issued (public client).

### Authorization Endpoint

`GET /authorize` with query parameters:

- `response_type=code`
- `client_id=<registered-client-id>`
- `redirect_uri=<registered-redirect-uri>`
- `state=<opaque-state>`
- `code_challenge=<S256-challenge>`
- `code_challenge_method=S256`

Returns an HTML page with three sign-in buttons:
- Sign in with Google
- Sign in with GitHub
- Sign in with Apple

Each button redirects to the respective provider's OAuth authorize URL. The `state` parameter encodes both the original MCP client state and the selected provider. The provider's callback redirect URI is set to `https://banana.yourdomain.com:8847/callback`.

### Provider Callback

`GET /callback` (Google, GitHub) or `POST /callback` (Apple — Apple sends a form POST) receives the provider's authorization code, exchanges it for a provider access token, fetches the user's email/profile, then:

1. Generates an MCP authorization code (random, 10-minute TTL)
2. Stores the code mapped to the user identity and original PKCE challenge
3. Redirects to the MCP client's `redirect_uri` with `code=<mcp-code>&state=<original-state>`

### Token Endpoint

`POST /token` handles two grant types:

**Authorization code exchange:**
- `grant_type=authorization_code`
- `code=<mcp-auth-code>`
- `client_id=<client-id>`
- `redirect_uri=<redirect-uri>`
- `code_verifier=<pkce-verifier>`

Validates the code, verifies the PKCE challenge, then returns:

```json
{
  "access_token": "<random-token>",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "<random-token>"
}
```

**Token refresh:**
- `grant_type=refresh_token`
- `refresh_token=<refresh-token>`
- `client_id=<client-id>`

Returns a new access token (1-hour TTL) and optionally rotates the refresh token.

### Token Validation in Middleware

The existing `authenticateRequest()` middleware in `internal/server/middleware.go` is extended:

1. Check bearer token against static `MCP_AUTH_TOKEN` / `MCP_AUTH_TOKENS_FILE` (existing flow)
2. If no match, check bearer token against the in-memory OAuth token store (new flow)
3. If neither matches, return 401

This means both auth methods work simultaneously.

### TLS Configuration

Two new env vars:

| Variable | Required | Description |
|---|---|---|
| `MCP_TLS_CERT_FILE` | No | Path to TLS certificate file (PEM) |
| `MCP_TLS_KEY_FILE` | No | Path to TLS private key file (PEM) |

When both are set, the server uses `http.ListenAndServeTLS`. When neither is set, the server uses plain HTTP (existing behavior). This allows local development without TLS.

### Provider Configuration

New env vars for each provider:

| Variable | Required | Description |
|---|---|---|
| `OAUTH_GOOGLE_CLIENT_ID` | No | Google OAuth client ID |
| `OAUTH_GOOGLE_CLIENT_SECRET` | No | Google OAuth client secret |
| `OAUTH_GITHUB_CLIENT_ID` | No | GitHub OAuth client ID |
| `OAUTH_GITHUB_CLIENT_SECRET` | No | GitHub OAuth client secret |
| `OAUTH_APPLE_CLIENT_ID` | No | Apple OAuth client ID |
| `OAUTH_APPLE_CLIENT_SECRET` | No | Apple OAuth client secret |
| `OAUTH_BASE_URL` | No | Base URL for OAuth endpoints (e.g., `https://banana.yourdomain.com:8847`) |

All are optional. Only providers with both client ID and secret configured are shown on the login page. If no providers are configured, OAuth is disabled and only bearer token auth works.

### Login Page

A minimal, self-contained HTML page embedded in the Go binary (via `embed`). Dark themed to match developer tooling. Shows only the configured providers. No external CSS/JS dependencies.

### In-Memory Store

```go
type Store struct {
    mutex            sync.RWMutex
    clients          map[string]*Client          // client_id -> client
    authCodes        map[string]*AuthCode        // code -> auth code data
    accessTokens     map[string]*TokenData       // token -> token data
    refreshTokens    map[string]*RefreshData     // token -> refresh data
    providerSessions map[string]*ProviderSession // state -> session data
}
```

All entries have TTLs. A background goroutine runs every 5 minutes to clean up expired entries.

### Security Considerations

- Authorization codes: 10-minute TTL, single-use
- Access tokens: 1-hour TTL
- Refresh tokens: 30-day TTL, rotated on use
- PKCE (S256) required for all authorization requests
- `state` parameter validated to prevent CSRF
- Redirect URIs must exactly match registered URIs
- Provider client secrets never appear in responses, logs, or error messages (added to existing sanitizer)
- All OAuth endpoints require HTTPS in production (enforced when TLS is configured)

### What Does NOT Change

- Existing bearer token auth (MCP_AUTH_TOKEN, MCP_AUTH_TOKENS_FILE)
- Stdio transport (no auth needed)
- MCP tool handlers (generate_image, edit_image, list_models, recommend_model)
- Rate limiting, concurrency, body size middleware
- Health check endpoint

## Testing Strategy

- Unit tests for PKCE verification, token generation, store TTL expiration
- Unit tests for each handler (metadata, register, authorize, callback, token)
- Integration test for full OAuth flow with mocked provider
- Existing middleware tests updated to verify OAuth token validation path
- Security tests: expired tokens rejected, invalid PKCE rejected, replay attacks blocked

## Files Changed

| File | Change |
|---|---|
| `internal/oauth/provider.go` | New — provider interface and configs |
| `internal/oauth/store.go` | New — in-memory token/client store |
| `internal/oauth/handler.go` | New — HTTP handlers |
| `internal/oauth/metadata.go` | New — server metadata response |
| `internal/oauth/registration.go` | New — dynamic client registration |
| `internal/oauth/pkce.go` | New — PKCE logic |
| `internal/oauth/login.html` | New — embedded login page |
| `internal/oauth/*_test.go` | New — tests for each file |
| `internal/config/config.go` | Modified — add OAuth + TLS env vars |
| `internal/server/server.go` | Modified — register OAuth routes |
| `internal/server/middleware.go` | Modified — add OAuth token validation |
| `internal/security/sanitizer.go` | Modified — register provider secrets |
| `cmd/mcp-banana/main.go` | Modified — TLS support |
| `.env.example` | Modified — add new env vars |
| `docker-compose.yml` | Modified — mount TLS cert volume |
| `README.md` | Modified — add OAuth/Claude Desktop quick start, update overview |
| `docs/claude-code-integration.md` | Modified — add Claude Desktop connector section |
| `docs/architecture.md` | Modified — add `internal/oauth/` to package layout, update request flow |
| `docs/authentication.md` | Modified — add OAuth as third auth option alongside bearer token and SSH |
| `docs/setup-and-operations.md` | Modified — add TLS setup, OAuth provider config, subdomain DNS steps |
| `docs/security.md` | Modified — add OAuth threat model, token TTLs, PKCE requirements |
| `docs/troubleshooting.md` | Modified — add OAuth-related troubleshooting entries |
| `docs/go-guide.md` | Modified — add Go concepts used: `embed` (login page), `crypto/rand` (token generation), `sync.RWMutex` (concurrent store), `net/http` TLS serving, `encoding/json` for OAuth payloads |
| `docs/root-files.md` | Modified — document any new root-level files if added |
| `CONTRIBUTING.md` | Modified — mention OAuth dev setup (provider test credentials) |
