# Security

## Overview

mcp-banana implements a defense-in-depth security model. Secrets are isolated in server memory and never cross any trust boundary. All user input is validated before reaching the Gemini API. Raw API errors are mapped to a strict allowlist of safe messages before being returned to Claude Code.

See [architecture.md](architecture.md) for security boundary diagrams and the middleware chain.

## Secret Isolation

At startup, both `GEMINI_API_KEY` and `MCP_AUTH_TOKEN` are registered with the sanitizer:

```go
security.RegisterSecret(serverConfig.GeminiAPIKey)
security.RegisterSecret(serverConfig.AuthToken)
```

`SanitizeString` applies three passes to any string before it is logged or returned:

1. Replace every registered secret value with `[REDACTED]`.
2. Replace any pattern matching `AIza[0-9A-Za-z_-]{35}` (the Gemini API key format) with `[REDACTED]`.
3. Strip `\n` and `\r` characters to prevent log injection.

The `sync.RWMutex` in `sanitize.go` makes these operations safe for concurrent use.

Additionally, `internal/gemini/registry.go` maintains two separate types:

- `ModelInfo` - contains the internal `GeminiID` field. Used only inside the `gemini` package.
- `SafeModelInfo` - deliberately excludes `GeminiID`. Used for all data returned to Claude Code.

No Gemini model ID ever appears in a tool response, log line, or error message.

## Input Validation

All user-supplied input is validated in `internal/security/validate.go` before being forwarded to the Gemini client.

| Validator | Input | Constraints |
|---|---|---|
| `ValidatePrompt` | `prompt` string | Non-empty; max 10,000 runes; no null bytes (`\x00`) |
| `ValidateModelAlias` | model alias string | Empty accepted (defaults to `nano-banana-2`); non-empty value must be in the registry allowlist |
| `ValidateAspectRatio` | aspect ratio string | Empty accepted; non-empty value must be one of: `1:1`, `16:9`, `9:16`, `4:3`, `3:4` |
| `ValidatePriority` | priority string | Empty accepted; non-empty value must be one of: `speed`, `quality`, `balanced` |
| `ValidateTaskDescription` | task description string | Non-empty; max 1,000 runes |
| `ValidateAndDecodeImage` | base64 image + MIME type | Non-empty; valid base64; decoded size within `MCP_MAX_IMAGE_BYTES`; MIME type must be `image/png`, `image/jpeg`, or `image/webp`; minimum 12 decoded bytes for magic byte validation; magic bytes must match the declared MIME type |

Magic byte validation checks the raw binary header of the decoded image:

- PNG: first 4 bytes are `\x89PNG`
- JPEG: first 3 bytes are `0xFF 0xD8 0xFF`
- WebP: bytes 0-3 are `RIFF` and bytes 8-11 are `WEBP`

This prevents a client from claiming a file is a PNG when it is not.

## Error Mapping Boundary

`internal/gemini/errors.go` is the critical security boundary between raw Gemini SDK errors and Claude Code. The raw `genai.APIError` type can contain request metadata, headers, and API keys in its message text. None of that text is ever forwarded.

The mapping works in two stages:

1. If the error is a `*genai.APIError`, extract only the HTTP status code and map it to a safe code.
2. Otherwise, classify the error by substring patterns in the message text (for classification only -- the matched text is discarded).

Five safe error codes form the complete allowlist:

| Code | Meaning |
|---|---|
| `content_policy_violation` | Prompt was blocked by Gemini content safety |
| `quota_exceeded` | API quota or rate limit exceeded |
| `model_unavailable` | The requested model is not found or deprecated |
| `generation_failed` | Image generation failed (safe to retry) |
| `server_error` | Internal server error |

No other error text may reach Claude Code.

## HTTP Error Contract

| Situation | HTTP Status | JSON Body |
|---|---|---|
| Missing or invalid bearer token | 401 | `{"error":"unauthorized"}` |
| Rate limit exceeded | 429 | `{"error":"rate_limited"}` |
| Request body exceeds 15 MB | 413 | `{"error":"request_too_large"}` |
| Request body read error | 400 | `{"error":"bad_request"}` |
| Server queue timeout (all slots busy) | 503 | `{"error":"server_busy"}` |
| Unexpected server error (panic) | 500 | `{"error":"server_error"}` |
| Health check | 200 | `{"status":"ok"}` |

## Threat Model

### Trust Boundaries

| Boundary | Authentication |
|---|---|
| Claude Code to mcp-banana (stdio) | Implicit: local process ownership |
| Claude Code to mcp-banana (HTTP) | Bearer token in `Authorization` header |
| mcp-banana to Gemini API | `GEMINI_API_KEY` in SDK configuration |
| mcp-banana to logs/responses | Sanitizer redacts secrets before output |

### What Is Protected

- **Google Gemini API key** - never included in any tool response, log line, or error message
- **Internal Gemini model IDs** - only the public Nano Banana aliases are exposed
- **Raw Gemini API errors** - translated to one of five safe error codes
- **Bearer token (HTTP mode)** - redacted from all log output

### What Is Not Protected

- **Prompt content** - prompts are forwarded to the Gemini API as-is. Do not include secrets in prompts.
- **Generated image content** - the server does not inspect or filter generated image data.
- **Claude Code's identity** - in stdio mode, the server trusts the local process. In HTTP mode, authentication is token-based, not identity-based.

### Security Controls at Each Boundary

| Control | Where Applied | Configurable |
|---|---|---|
| Bearer token auth | HTTP middleware, every request except `/healthz` | `MCP_AUTH_TOKEN` / `MCP_AUTH_TOKENS_FILE` |
| Rate limiting | HTTP middleware | `MCP_RATE_LIMIT` |
| Global concurrency limit | HTTP middleware | `MCP_GLOBAL_CONCURRENCY` |
| Pro-model concurrency limit | Gemini client, semaphore | `MCP_PRO_CONCURRENCY` |
| Body size limit | HTTP middleware | Fixed at 15 MB |
| Request timeout | Gemini client, context deadline | `MCP_REQUEST_TIMEOUT_SECS` |
| Input validation | Tool handlers, security package | Fixed constraints |
| Secret sanitization | Startup registration + all log/response paths | Automatic |
| Error mapping | Gemini client and tool handlers | Fixed allowlist |
| Panic recovery | HTTP middleware | Always active |
| Registry sentinel check | Startup, `ValidateRegistryAtStartup()` | Blocks boot if triggered |
