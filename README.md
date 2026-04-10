# mcp-banana

A Go MCP server wrapping Google's Gemini image generation API for Claude Code.

## Overview

**mcp-banana** is an MCP (Model Context Protocol) server that provides Claude Code with access to Google's Gemini image generation and editing capabilities. It implements a security-first architecture with server-side secret isolation, input validation, and output sanitization.

The server supports two transports:
- **Stdio** — for local development (runs in-process with Claude Code)
- **HTTP** — for remote deployment (standalone server with bearer token authentication)

Four MCP tools are exposed:
1. **generate_image** — Create images from text prompts
2. **edit_image** — Modify existing images with text instructions
3. **list_models** — Enumerate all available model aliases and capabilities
4. **recommend_model** — Get a model recommendation based on task description and priority

## Architecture Overview

```
Claude Code
    |
    +----- [stdio transport] -----+
    |                             |
    +---- [HTTP + Bearer Token] --+
                                  |
                         mcp-banana Server
                         /        |       \
                    tools/      config/   security/
                      |            |         |
                    generate_     Config   Sanitize
                    edit_list_    Load     Validate
                    recommend_                |
                              gemini/
                              /    |    \
                          Client  Service Registry
                             |       |        |
                          Gemini API (with timeout & rate limiting)
```

**Package Layout:**
- `cmd/mcp-banana/main.go` — Entry point; parses transport flags and starts server
- `internal/config/` — Loads and validates environment variables; holds secrets at runtime
- `internal/gemini/` — Gemini API client, model registry, error handling
- `internal/security/` — Secret sanitization and input validation
- `internal/server/` — HTTP routing, middleware (auth, rate limiting, concurrency), health checks
- `internal/tools/` — MCP tool handlers (generate, edit, list, recommend)

## Prerequisites

- **Go 1.24+** — required to build from source
- **golangci-lint** — required for development (code linting)
- **Docker** — recommended for remote deployment
- **SSH access** — required to deploy to DigitalOcean (if using remote deployment)

## Quick Start

### Build and Run Locally

1. **Set environment variables:**
   ```bash
   export GEMINI_API_KEY="your-google-gemini-api-key"
   export MCP_LOG_LEVEL="info"
   ```

2. **Build the server:**
   ```bash
   make build
   ```

3. **Start in stdio mode (for Claude Code integration):**
   ```bash
   make run-stdio
   ```

   Or manually:
   ```bash
   ./mcp-banana --transport stdio
   ```

The server will start and listen for JSON-RPC calls on stdin/stdout. Claude Code can connect to this process directly.

## Configuration

All configuration is loaded from environment variables at startup. If a required variable is missing or malformed, the server exits immediately with an error.

| Variable | Default | Description |
|---|---|---|
| `GEMINI_API_KEY` | *(required)* | Google Gemini API key for authentication |
| `MCP_AUTH_TOKEN` | *(optional)* | Bearer token for HTTP transport; required if transport is HTTP |
| `MCP_LOG_LEVEL` | `info` | Logging verbosity: `debug`, `info`, `warn`, or `error` |
| `MCP_RATE_LIMIT` | 30 | Maximum requests per minute across all models |
| `MCP_GLOBAL_CONCURRENCY` | 8 | Maximum simultaneous requests across all models |
| `MCP_PRO_CONCURRENCY` | 3 | Maximum simultaneous requests reserved for the Pro model |
| `MCP_MAX_IMAGE_BYTES` | 4194304 (4 MB) | Maximum size of decoded image input in bytes |
| `MCP_REQUEST_TIMEOUT_SECS` | 120 | Timeout for each Gemini API call in seconds |

## Security Model

mcp-banana implements a defense-in-depth security model with multiple layers:

### Secret Isolation
- **API keys and auth tokens are never included in tool responses, logs, or error messages.**
- At startup, secrets are registered with the sanitizer via `security.RegisterSecret()`.
- All output (success responses, error messages, logs) is scanned and secrets are redacted before sending to Claude Code.

### Input Validation
- **All user input is validated before reaching the Gemini API:**
  - Model aliases are checked against the allowlist in `internal/gemini/registry.go`
  - Image data is validated (magic bytes, size limits) in `internal/security/validate.go`
  - Prompt and instruction text lengths are bounded
  - MIME types are validated

### Output Sanitization
- **Errors from the Gemini API are mapped to safe, generic error messages.**
- Raw Gemini error details are logged (redacted) but never returned to Claude Code.
- Registered secrets are redacted from all log output and error responses.

### HTTP Bearer Token Authentication
- **For HTTP transport, every request (except `/healthz`) requires a valid bearer token.**
- Token is passed in the `Authorization` header: `Bearer <token>`
- Invalid or missing tokens result in a 401 Unauthorized response.
- Tokens are registered as secrets and redacted from logs.

## Threat Model

### What Is Protected
- **Google Gemini API keys** — never exposed to Claude Code or logs
- **Internal Gemini model IDs** — only the user-facing aliases are exposed
- **Raw Gemini API errors** — translated to safe, generic messages
- **Bearer tokens (HTTP)** — redacted from logs and responses

### Trust Boundaries
1. **Claude Code ↔ MCP Server:** Authenticated via stdio (local) or bearer token (HTTP)
2. **MCP Server ↔ Gemini API:** Authenticated via API key (secret); never exposed to Claude Code
3. **MCP Server ↔ Logs/Responses:** All secrets redacted by sanitizer before output

### Security Controls at Each Boundary
- **Input Validation:** All user input validated before Gemini API calls
- **Rate Limiting:** Prevents abuse; configured per environment
- **Concurrency Limits:** Protects Gemini API from overload; enforced via semaphore
- **Body Size Limits:** HTTP requests capped at 15 MB to prevent memory exhaustion
- **Request Timeouts:** Each Gemini API call has a configurable timeout

## HTTP Error Contract

When running in HTTP mode, the server returns JSON-formatted error responses with specific status codes. The MCP protocol uses JSON-RPC, and normal tool calls POST to `/mcp`.

| Situation | HTTP Status | JSON Body |
|---|---|---|
| Missing or invalid bearer token | 401 | `{"error":"unauthorized"}` |
| Rate limit exceeded | 429 | `{"error":"rate_limited"}` |
| Request body exceeds 15 MB | 413 | `{"error":"request_too_large"}` |
| Request body read error | 400 | `{"error":"bad_request"}` |
| Server queue timeout (all slots busy) | 503 | `{"error":"server_busy"}` |
| Unexpected server error | 500 | `{"error":"server_error"}` |
| Health check endpoint | 200 | `{"status":"ok"}` |

**Normal flow:** Claude Code sends a JSON-RPC request to `POST /mcp` with the bearer token. The request is validated, rate-limited, and queued. If successful, the MCP server processes the tool call and returns a JSON-RPC response.

## Models

Three model aliases are available. Each maps to a Gemini model ID and offers different speed/quality tradeoffs.

| Alias | Description | Typical Latency | Best For |
|---|---|---|---|
| `nano-banana-2` | Fast, high-volume generation | 5–10s | Iterative work, drafts, batch generation |
| `nano-banana-pro` | Professional quality with advanced reasoning | 15–45s | Final assets, photorealistic images, complex scenes |
| `nano-banana-original` | Speed and efficiency optimized | 3–8s | Quick previews, high-volume batch work |

All models support both `generate` and `edit` operations.

### Model ID Verification Status

The model IDs in `internal/gemini/registry.go` currently use sentinel values (`VERIFY_MODEL_ID_BEFORE_RELEASE`). This is intentional and prevents accidental deployment with unverified IDs.

**Status:** The server is implementation-ready but **release-blocked** until the Gemini model IDs are verified against the live Gemini API.

Before public release, verify each model alias by calling the [Gemini models.list API](https://ai.google.dev/gemini-api/docs/models) or checking the official documentation. Once verified, replace the sentinel values in `internal/gemini/registry.go` with the actual model IDs (e.g., `gemini-3.1-flash-image-preview`).

**Expected mappings to verify:**
```
nano-banana-2        -> gemini-3.1-flash-image-preview
nano-banana-pro      -> gemini-3-pro-image-preview
nano-banana-original -> gemini-2.5-flash-image (or similar)
```

If a confirmed model ID cannot be found for an alias, remove that alias from the registry before release.

### Startup Validation

The function `gemini.ValidateRegistryAtStartup()` runs during server startup and blocks boot if any sentinel values are still present. The server will exit with:
```
registry validation failed: model "nano-banana-2" has unverified GeminiID -- verify at https://ai.google.dev/gemini-api/docs/models before release
```

**This is expected behavior, not a bug.** Fix it by updating `internal/gemini/registry.go` with verified model IDs.

In a Docker environment, this will cause the container to exit or remain unhealthy. The container will not serve traffic until the model IDs are verified.

## Development

### Build

```bash
make build
```

Compiles the binary to `./mcp-banana` with optimizations (`-ldflags="-s -w"` and `-trimpath`).

### Test

```bash
make test
```

Runs all unit and integration tests with race detection and coverage reporting. Coverage data is written to `coverage.out`.

### Lint

```bash
make lint
```

Runs `golangci-lint run` to check for code quality issues.

### Format

```bash
make fmt
```

Formats all Go code with `gofmt -w .`.

Check (without writing) with:
```bash
make fmt-check
```

### Vet

```bash
make vet
```

Runs `go vet ./...` for static analysis.

### Quality Gate

```bash
make quality-gate
```

Runs the full CI sequence in order:
1. `golangci-lint run` (lint)
2. `gofmt -w .` (format)
3. `go vet ./...` (static analysis)
4. `go test ./...` (tests)

All checks must pass before committing. Fix issues iteratively and re-run until all steps are green.

## Deployment

### Docker

The project includes a `docker-compose.yml` for deployment to DigitalOcean or other Docker-based environments.

1. **SSH to your server:**
   ```bash
   ssh root@your-server-ip
   ```

2. **Clone the repository:**
   ```bash
   git clone https://github.com/reshinto/mcp-banana.git /opt/mcp-banana
   cd /opt/mcp-banana
   ```

3. **Configure environment:**
   ```bash
   cp .env.example .env
   # Edit .env with your actual values:
   # - GEMINI_API_KEY (from Google)
   # - MCP_AUTH_TOKEN (generate with: openssl rand -hex 32)
   nano .env
   ```

4. **Start the server:**
   ```bash
   docker compose up -d --build
   ```

   The server listens on `0.0.0.0:8847` by default.

5. **Verify health:**
   ```bash
   curl http://localhost:8847/healthz
   ```

   Expected response:
   ```json
   {"status":"ok"}
   ```

### Token Rotation

To rotate the MCP auth token:

```bash
make rotate-token
```

This generates a new token and prints instructions for updating both the server and your Claude Code configuration.

## CI/CD Pipeline

The project uses GitHub Actions for continuous integration and continuous deployment.

### Continuous Integration

CI runs automatically on:
- **Pushes to feature/fix/chore branches** — validates code quality, tests, and formatting before review
- **Pull requests to main** — ensures quality gates pass before merge
- **Pushes to main** — CD workflow invokes CI before deploying to production

The CI sequence runs in this order and must pass completely:
1. `golangci-lint run` — code linting
2. `gofmt -w .` — code formatting
3. `go vet ./...` — static analysis
4. `go test ./...` — unit and integration tests

### Continuous Deployment

CD runs automatically on **pushes to main** and:
1. **Builds and publishes a Docker image** to the container registry
2. **Deploys to DigitalOcean** with the new image
3. **Runs smoke tests** against the deployed server to validate basic functionality
4. **Auto-rollback** if post-deploy health checks fail; manual intervention required only if rollback also fails

### Secrets Management

**Deployment secrets** (Docker registry credentials, DigitalOcean API tokens, deployment keys) are stored as **GitHub environment secrets**, not in code or configuration files. Only the `.env.example` template is committed to the repository.

**Application runtime secrets** (Gemini API key, MCP auth token) live only on the server in the `.env` file. They are never stored in GitHub or version control. See the [Deployment](#deployment) section for setup instructions.

### Rollback Policy

- **Automatic rollback:** If post-deploy health checks detect the new deployment is unhealthy, the system automatically rolls back to the previous version.
- **Manual rollback:** If automatic rollback fails, manual intervention is required. Contact the DevOps team or re-run the deployment workflow with the previous image tag.

## Using with Claude Code

mcp-banana can be integrated with Claude Code as a local or remote MCP server. Choose the setup that fits your team's workflow.

### Team Adoption Recommendation

| Scope | Use case | How |
|---|---|---|
| **User scope** (`--scope user`) | Personal local development | `claude mcp add --scope user ...` (stored in `~/.claude.json`) |
| **Project scope** (`--scope project`) | Shared team configuration | `claude mcp add --scope project ...` (stored in `.mcp.json`) |
| **HTTP** | Preferred remote transport | Direct HTTP or SSH-tunneled HTTP |
| **SSE** | Deprecated | Do not use |

### Option A: Local stdio Mode (Recommended for Development)

**Prerequisites:**
- Build the binary: `make build`
- Get a Google Gemini API key from [ai.google.dev](https://ai.google.dev)

**User-scoped setup** (secrets stored locally):
```bash
claude mcp add-json --scope user banana '{
  "command": "/usr/local/bin/mcp-banana",
  "args": ["--transport", "stdio"],
  "env": {
    "GEMINI_API_KEY": "<your-gemini-api-key>"
  },
  "type": "stdio"
}'
```

**Project-scoped setup** (committed to repo, secret-free):
```bash
claude mcp add-json --scope project banana '{
  "command": "${MCP_BANANA_BIN:-mcp-banana}",
  "args": ["--transport", "stdio"],
  "type": "stdio"
}'
```

**Note:** The project-scoped config contains no secrets. Each developer supplies their own `GEMINI_API_KEY` via user-scoped config or environment variable.

The `.mcp.json` file is already committed to this repository with the project-scoped configuration above.

### Option B: Remote HTTP Mode (For Deployed Server)

Connect to a remotely deployed instance:

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://localhost:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-mcp-auth-token>"
  }
}'
```

**Optional SSH tunnel hardening:**
```bash
ssh -N -L 8847:127.0.0.1:8847 user@<droplet-ip>
```

Then update the URL to `http://localhost:8847/mcp` in the config above.

### Verification

After adding the server:
```bash
claude mcp list
claude mcp get banana
```

Both commands should show the banana server configuration.

### Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `claude mcp list` does not show banana | Server not added | Re-run `claude mcp add-json` with correct scope |
| "GEMINI_API_KEY is required" | API key not set | Add `env` block to user-scoped config or set environment variable |
| "model registry validation failed" | Sentinel IDs present | Replace `VERIFY_MODEL_ID_BEFORE_RELEASE` in `internal/gemini/registry.go` with verified Gemini model IDs |
| HTTP "unauthorized" (401) | Wrong or missing auth token | Verify `MCP_AUTH_TOKEN` matches the token in server `.env` |
| "Connection refused" | SSH tunnel not running or server down | Start SSH tunnel or verify remote server is healthy with `curl http://localhost:8847/healthz` |

**Pre-release note:** The server refuses to start with sentinel model IDs in the registry. This is intentional to prevent accidental deployment with unverified IDs. See the [Model ID Verification Status](#model-id-verification-status) section for details.

## Go Glossary

For contributors unfamiliar with Go, here are common abbreviations used in this codebase:

| Abbreviation | Meaning | Explanation |
|---|---|---|
| `err` | error | Holds the error returned by the previous operation. In Go, functions return errors as values (not exceptions), so error handling is explicit. Example: `if err != nil { return err }` |
| `ctx` | context | Carries request-scoped deadlines, cancellation signals, and metadata across API boundaries. Essential for managing timeouts and coordinating shutdown. Example: `ctx, cancel := context.WithTimeout(...)` |
| `req` | request | The incoming HTTP request from the client, or a generic request structure. Contains headers, body, query parameters, etc. |
| `resp` | response | The HTTP response received from an upstream service (e.g., Gemini API). Contains status code, headers, and body. |
| `cfg` | config | The parsed application configuration (from environment variables in this project). Contains database credentials, API keys, and tuning parameters. |
| `srv` | server | The HTTP server instance that listens for incoming connections. In this project: `*http.Server` or `*mcpserver.MCPServer` |
| `test` | test | The Go test runner (`*testing.T`) that provides logging and failure reporting. Used in all unit tests. Example: `func TestFoo(test *testing.T) { ... }` |

## License

[Add your license here]

## Support

For issues or questions, please open an issue on GitHub or contact the maintainers.
