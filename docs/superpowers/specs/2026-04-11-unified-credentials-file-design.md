# Unified Credentials File

**Date:** 2026-04-11
**Status:** Draft
**Scope:** Replace separate auth token and Gemini API key configuration with a single hot-reloadable credentials file.

## Problem

The current setup requires restarting the Docker container to change the Gemini API key or auth tokens. Auth tokens and Gemini API keys are configured separately, with no link between a client's identity and their Gemini key. OAuth cannot protect the server on its own — it requires a static token to be configured alongside it.

## Solution

Introduce `MCP_CREDENTIALS_FILE`, a JSON file that maps client identities (bearer tokens or OAuth identities) to Gemini API keys. The file is hot-reloaded on every request, so credentials can be updated without restarting the container.

## Env Var Changes

### Added

- `MCP_CREDENTIALS_FILE` — path to the unified credentials JSON file

### Removed

- `GEMINI_API_KEY` — replaced by per-client keys in the credentials file
- `MCP_AUTH_TOKEN` — replaced by bearer token entries in the credentials file
- `MCP_AUTH_TOKENS_FILE` — replaced by the credentials file

### Kept

- `OAUTH_GOOGLE_CLIENT_ID`, `OAUTH_GOOGLE_CLIENT_SECRET`
- `OAUTH_GITHUB_CLIENT_ID`, `OAUTH_GITHUB_CLIENT_SECRET`
- `OAUTH_APPLE_CLIENT_ID`, `OAUTH_APPLE_CLIENT_SECRET`
- `OAUTH_BASE_URL`
- `MCP_DOMAIN`
- `MCP_LOG_LEVEL`, `MCP_RATE_LIMIT`, `MCP_GLOBAL_CONCURRENCY`, `MCP_PRO_CONCURRENCY`
- `MCP_MAX_IMAGE_BYTES`, `MCP_REQUEST_TIMEOUT_SECS`
- `MCP_TLS_CERT_FILE`, `MCP_TLS_KEY_FILE`

## Credentials File Format

```json
{
  "google:user@gmail.com": "AIzaSy...",
  "github:devuser": "AIzaSy...",
  "abc123staticbearertoken": "AIzaSy..."
}
```

- Keys are either bearer tokens (plain strings) or OAuth identities (`provider:email` format).
- Values are Gemini API keys.
- File is re-read on every request for hot-reload.
- Server creates the file automatically if it does not exist.
- Server pre-checks whether the file exists on startup.

## Auth + Key Resolution Flow

On each incoming request, the server resolves authentication and the Gemini API key in this order:

### Priority 1: OAuth Access Token

If the request's bearer token is a valid OAuth access token:
- Resolve the token to the user's identity (`provider:email`).
- Look up the identity in the credentials file.
- If found, use the mapped Gemini API key.
- If not found, reject (user must complete OAuth onboarding flow with Gemini key prompt).

### Priority 2: Static Bearer Token

If the request's bearer token matches a key in the credentials file:
- Use the mapped Gemini API key.

### Priority 3: Self-Registration

If the bearer token is unknown AND the request includes an `X-Gemini-API-Key` header:
- Write/overwrite `"token": "gemini_key"` in the credentials file.
- Proceed with the provided Gemini key.

### Priority 4: Rejection

If the bearer token is unknown and no `X-Gemini-API-Key` header is present:
- Reject the request (401 Unauthorized).

## Self-Registration via `X-Gemini-API-Key` Header

The `X-Gemini-API-Key` header serves as a one-time registration mechanism:

**First request with a new bearer token:**
1. Client sends `Authorization: Bearer <token>` + `X-Gemini-API-Key: AIza...`
2. Server registers `"token": "AIza..."` in the credentials file.
3. Request proceeds with the provided Gemini key.

**Subsequent requests:**
1. Client sends only `Authorization: Bearer <token>`.
2. Server finds the token in the credentials file, retrieves the Gemini key.

**Key rotation:**
If a client sends `X-Gemini-API-Key` with a token that already exists in the file, the Gemini key is overwritten (allows rotation).

**Validation:**
An empty string `X-Gemini-API-Key` header is rejected — the server treats it as if the header was not sent.

## OAuth Onboarding Flow

### First-time user (identity not in credentials file)

1. User authenticates via Google, GitHub, or Apple.
2. Server prompts the user to enter their Gemini API key. **This is required — the user cannot skip this step.** Empty string input is rejected; the user must provide a non-empty key.
3. Server writes `"provider:email": "gemini_api_key"` to the credentials file.
4. Request proceeds with the provided Gemini key.

### Returning user (identity already in credentials file)

1. User authenticates via Google, GitHub, or Apple.
2. Server prompts the user to enter their Gemini API key, with an option to **skip**.
3. If the user provides a new non-empty key, the existing entry is overwritten (allows key rotation). Empty string input is rejected.
4. If the user skips, the existing Gemini key is used unchanged.
5. Request proceeds.

## Gemini API Key Validation

When a Gemini API key is being written to the credentials file (registration or update), the server validates the key by making a lightweight Gemini API call before accepting it.

- If validation succeeds, the key is written to the file and the request proceeds.
- If validation fails, the key is rejected, nothing is written, and the request fails with an error indicating the Gemini key is invalid.

Validation occurs at write time only:
1. Self-registration via `X-Gemini-API-Key` header (new or updated key).
2. OAuth onboarding (first-time or returning user entering a new key).

Existing keys in the file are not re-validated on every request. If a key is revoked later, the Gemini API call itself will fail with a clear error at request time.

## Auth Guard

Authentication is enforced if ANY of the following are true:
- `MCP_CREDENTIALS_FILE` is set
- OAuth is configured (any provider has both client ID and secret)

If none are configured, all requests pass through (SSH tunnel mode).

## Security Considerations

- Gemini API keys read from the credentials file must be registered with `security.RegisterSecret()` so they are redacted from logs and error output.
- The credentials file should have restrictive file permissions (0600).
- The self-registration endpoint must not leak whether a token already exists (always return the same response shape).
- OAuth client secrets remain in env vars — they are not moved to the credentials file.

## File Management

- The server checks for the file's existence on startup.
- If the file does not exist and the path is set, the server creates it with an empty JSON object (`{}`).
- File writes must be atomic (write to temp file, rename) to prevent corruption from concurrent requests.
- File reads on every request: read the file, parse JSON, look up the key. This is acceptable for a small file.

## Affected Packages

| Package | Changes |
|---|---|
| `internal/config/` | Add `CredentialsFile` field, remove `GeminiAPIKey`, `AuthToken`, `AuthTokensFile` |
| `internal/server/middleware.go` | Rewrite `authenticateRequest` with new priority order, add self-registration logic |
| `internal/gemini/cache.go` | Remove default client concept; all clients are per-key from credentials file |
| `internal/tools/` | Tool handlers resolve Gemini key from credentials file instead of default client |
| `cmd/mcp-banana/main.go` | Remove `GEMINI_API_KEY` client creation, pass credentials file path |
| `internal/oauth/` | Add Gemini key prompt step to OAuth onboarding flow |

## Testing

- Credentials file CRUD: read, write, overwrite, create-if-missing
- Auth priority: OAuth > static token > self-registration > rejection
- Self-registration: new token + header registers, existing token + header overwrites
- Hot-reload: file changes picked up on next request without restart
- OAuth onboarding: identity written to file after Gemini key prompt
- Security: Gemini keys from file registered with sanitizer, not leaked in errors
- Edge cases: empty file, malformed JSON, concurrent writes, missing file path

## Migration

This is a breaking change. Users must:
1. Create a credentials file mapping their existing tokens to Gemini keys.
2. Set `MCP_CREDENTIALS_FILE` in their `.env`.
3. Remove `GEMINI_API_KEY`, `MCP_AUTH_TOKEN`, and `MCP_AUTH_TOKENS_FILE` from `.env`.
