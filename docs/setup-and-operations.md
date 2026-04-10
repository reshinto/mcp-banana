# Setup and Operations

## Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.26 or later | Build from source |
| golangci-lint | v2.11.4 or later | Code linting (development) |
| Docker | Any recent version | Container deployment |
| Docker Compose | v2 | Multi-container deployment |
| OpenSSL | Any | Generating auth tokens |
| SSH | Any | Remote deployment access |

Install golangci-lint:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.11.4
```

## Local Development Setup

**Step 1: Get a Gemini API key**

Visit [https://aistudio.google.com/](https://aistudio.google.com/), sign in, and create an API key. The key starts with `AIza`.

**Step 2: Clone and build**

```bash
git clone https://github.com/reshinto/mcp-banana.git
cd mcp-banana
make build
```

**Step 3: Set environment variables**

```bash
export GEMINI_API_KEY="AIza..."
```

**Step 4: Replace sentinel model IDs**

Before the server can start, the model IDs in `internal/gemini/registry.go` must be verified. See [models.md](models.md) for the procedure.

**Step 5: Run and verify**

```bash
make run-stdio
# or: ./mcp-banana --transport stdio
```

Run the quality gate before committing any changes:

```bash
make quality-gate
```

## Configuration Reference

All configuration is loaded from environment variables at startup. Missing required variables or malformed optional values cause an immediate exit with a descriptive error.

| Variable | Required | Default | Description |
|---|---|---|---|
| `GEMINI_API_KEY` | Yes | - | Gemini API key; registered as a secret |
| `MCP_AUTH_TOKEN` | No | - | Single bearer token for HTTP auth; registered as a secret |
| `MCP_AUTH_TOKENS_FILE` | No | - | Path to a file with one token per line; hot-reloaded on every request |
| `MCP_LOG_LEVEL` | No | `info` | One of: `debug`, `info`, `warn`, `error` |
| `MCP_RATE_LIMIT` | No | `30` | Positive integer; requests per minute |
| `MCP_GLOBAL_CONCURRENCY` | No | `8` | Positive integer; max simultaneous in-flight requests |
| `MCP_PRO_CONCURRENCY` | No | `3` | Positive integer; must be <= `MCP_GLOBAL_CONCURRENCY` |
| `MCP_MAX_IMAGE_BYTES` | No | `4194304` | Positive integer; decoded image size limit in bytes (default 4 MB) |
| `MCP_REQUEST_TIMEOUT_SECS` | No | `120` | Positive integer; per-call Gemini API timeout in seconds |

`MCP_PRO_CONCURRENCY` must be <= `MCP_GLOBAL_CONCURRENCY`. The server exits at startup if this is violated.

Copy the env template:

```bash
cp .env.example .env
# Edit .env with your actual values
```

The `.env` file is listed in `.gitignore` and must never be committed.

See [authentication.md](authentication.md) for how `MCP_AUTH_TOKEN` and `MCP_AUTH_TOKENS_FILE` work.

## Production Deployment (Docker)

**Step 1: Provision a server**

Create a server (Ubuntu 22.04 LTS recommended, 1 GB RAM minimum) with SSH key authentication.

**Step 2: Install Docker**

```bash
ssh root@<server-ip>
apt-get update && apt-get install -y docker.io docker-compose-plugin
systemctl enable docker && systemctl start docker
```

**Step 3: Create a deploy user**

```bash
adduser deploy
usermod -aG docker deploy
# Add your SSH public key to /home/deploy/.ssh/authorized_keys
```

**Step 4: Clone the repository**

```bash
sudo git clone https://github.com/reshinto/mcp-banana.git /opt/mcp-banana
sudo chown -R deploy:deploy /opt/mcp-banana
```

**Step 5: Configure environment**

```bash
cd /opt/mcp-banana
cp .env.example .env
nano .env
# Set at minimum: GEMINI_API_KEY and MCP_AUTH_TOKEN
```

Docker Compose reads `.env` via `env_file: .env` in `docker-compose.yml` and injects all variables into the container. The `.env` file is not tracked by git, so `git pull` will not overwrite it.

**Step 6: Verify model IDs**

See [models.md](models.md) to replace sentinel IDs before deploying.

**Step 7: Build and start**

```bash
docker compose up -d --build
```

The container:
- Runs in HTTP mode on `0.0.0.0:8847` inside the container
- Binds to `127.0.0.1:8847` on the host (loopback only, not public)
- Restarts automatically on failure (`restart: unless-stopped`)
- Has a 120-second graceful shutdown period (`stop_grace_period: 120s`)
- Is limited to 768 MB of memory

**Step 8: Verify health**

```bash
curl http://localhost:8847/healthz
# Expected: {"status":"ok"}
docker compose ps
```

If the container shows `unhealthy`, check logs:

```bash
docker compose logs mcp-banana
```

Common startup failures:

| Error | Cause | Fix |
|---|---|---|
| `GEMINI_API_KEY is required` | `.env` missing or key is empty | Check `.env` file |
| `model registry validation failed` | Sentinel IDs not replaced | See [models.md](models.md) |
| `MCP_AUTH_TOKEN is required for HTTP transport` | Token not set | Add `MCP_AUTH_TOKEN` to `.env` |

### Updating Production

```bash
ssh deploy@<server-ip>
cd /opt/mcp-banana
git pull origin main
docker compose up -d --build --force-recreate
curl http://127.0.0.1:8847/healthz
```

Roll back if the new version fails:

```bash
git checkout <previous-commit-sha>
docker compose up -d --build --force-recreate
```

## CI Pipeline

CI runs automatically via GitHub Actions (`.github/workflows/ci.yml`) on pushes to `main`, `feat/**`, `fix/**`, `chore/**` branches and on pull requests to `main`.

Steps:

1. `golangci-lint run` (5-minute timeout)
2. `gofmt -l .` format check
3. `go vet ./...` static analysis
4. `go test -coverprofile=coverage.out -race ./...` with 80% coverage threshold
5. Binary build (`CGO_ENABLED=0`, `-ldflags="-s -w"`, `-trimpath`)
6. Binary size check (15 MB limit)
7. Docker image build
8. Docker image size check (25 MB limit)

Deployment to production is manual via SSH -- there is no automated CD pipeline.

## Monitoring

### Health Endpoint

```bash
curl http://localhost:8847/healthz
# Returns {"status":"ok"} with HTTP 200
```

The container runs an internal health check every 30 seconds. After 3 consecutive failures, Docker marks the container `unhealthy`.

### Logs

Logs are written as JSON to stderr, captured by Docker's `json-file` driver (10 MB cap, 3 files of rotation):

```bash
docker compose logs -f mcp-banana
```

| Level | When |
|---|---|
| `debug` | Detailed request tracing (development only) |
| `info` | Normal startup and request events (default) |
| `warn` | Unexpected but recoverable conditions |
| `error` | Failures requiring attention |

Set `MCP_LOG_LEVEL=debug` temporarily to diagnose issues. Revert to `info` for production.

### Token Rotation

```bash
make rotate-token
```

Generates a new random token and prints step-by-step instructions for updating the server and your Claude Code configuration.

## End-to-End Flow: From Prompt to Image

This section traces exactly what happens when you ask Claude Code to generate an image, from the moment you type the prompt to the moment you see the result.

### Step 1: You ask Claude Code

You type in Claude Code:

> "Generate an image of a red apple on a white background"

Claude Code recognizes this as an image generation task and decides to call the `generate_image` tool on the `banana` MCP server.

### Step 2: Claude Code sends JSON-RPC to the MCP server

Claude Code constructs a JSON-RPC request and sends it to the server:

- **Stdio mode**: writes to the mcp-banana process's stdin
- **HTTP mode**: sends `POST /mcp` with the bearer token in the `Authorization` header

The JSON-RPC payload looks like:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "generate_image",
    "arguments": {
      "prompt": "A red apple on a white background",
      "model": "nano-banana-2"
    }
  }
}
```

### Step 3: Middleware processes the request (HTTP mode only)

In HTTP mode, the request passes through the middleware chain before reaching the tool handler. In stdio mode, this step is skipped entirely.

1. **Panic recovery** -- wraps everything in a safety net
2. **Health check bypass** -- not `/healthz`, so continue
3. **Bearer token auth** -- checks the token against `MCP_AUTH_TOKEN` or `MCP_AUTH_TOKENS_FILE`
4. **Rate limiter** -- checks if the request is within the rate limit (default 30/min)
5. **Concurrency semaphore** -- acquires a slot (waits up to 5 seconds if all slots are busy)
6. **Body size check** -- reads the body, rejects if > 15 MB

If any check fails, the middleware returns an error immediately (401, 429, 503, or 413) and the request never reaches the tool handler.

Source: `internal/server/middleware.go`

### Step 4: Tool handler validates input

The `generate_image` handler (`internal/tools/generate.go`) extracts the arguments and validates each one:

| Field | Validation | Source |
|---|---|---|
| `prompt` | Non-empty, max 10,000 runes, no null bytes | `security.ValidatePrompt()` |
| `model` | Must be in the registry allowlist, or empty (defaults to `nano-banana-2`) | `security.ValidateModelAlias()` |
| `aspect_ratio` | Must be `1:1`, `16:9`, `9:16`, `4:3`, `3:4`, or empty | `security.ValidateAspectRatio()` |

If validation fails, the handler returns an error result like `"invalid_prompt: prompt is required"` directly to Claude Code. The Go error return is always `nil` -- all errors are encoded in the MCP tool result.

Source: `internal/security/validate.go`

### Step 5: Gemini client prepares the API call

The handler calls `service.GenerateImage(ctx, "nano-banana-2", "A red apple on a white background", options)`.

Inside the Gemini client (`internal/gemini/client.go`):

1. **Model lookup** -- `LookupModel("nano-banana-2")` returns the internal `ModelInfo` with the real Gemini model ID (e.g., `gemini-3.1-flash-image-preview`). This Gemini ID is never exposed to Claude Code.

2. **Pro semaphore** -- if the model is `nano-banana-pro`, the client acquires a slot from the pro semaphore (limited to `MCP_PRO_CONCURRENCY`, default 3). For other models, this step is skipped.

3. **Timeout** -- wraps the request context with a deadline of `MCP_REQUEST_TIMEOUT_SECS` (default 120 seconds).

4. **Build request** -- `buildGenerateInputs()` constructs the Gemini API call:
   - Model name: the real Gemini ID
   - Content: the prompt as a text part
   - Config: requests `image/png` as the response MIME type

Source: `internal/gemini/client.go`, `internal/gemini/registry.go`

### Step 6: Gemini API generates the image

The client calls `client.inner.Models.GenerateContent(ctx, modelName, contents, config)`.

This is a network call to Google's Gemini API. The API key is sent in the request header by the SDK. The call typically takes:

| Model | Typical Wait |
|---|---|
| `nano-banana-original` | 3-8 seconds |
| `nano-banana-2` | 5-10 seconds |
| `nano-banana-pro` | 15-45 seconds |

If the call fails (network error, quota exceeded, content policy violation), the error is caught in the next step.

### Step 7: Response is extracted and validated

`extractImage()` in `internal/gemini/client.go` processes the Gemini response:

1. Checks that the response has candidates (no candidates = content policy block)
2. Finds the first `InlineData` part with image data
3. Validates the MIME type is one of `image/png`, `image/jpeg`, `image/webp`
4. Base64-encodes the raw image bytes
5. Records the generation time in milliseconds
6. Returns an `ImageResult` with the Nano Banana alias (not the Gemini ID) in `model_used`

If anything goes wrong, `MapError()` in `internal/gemini/errors.go` converts the raw error to a safe error code. The raw Gemini error text is **never** returned to Claude Code.

### Step 8: Handler returns the result to Claude Code

The handler JSON-serializes the `ImageResult` and wraps it in an MCP tool result:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"image_base64\":\"iVBORw0KGgo...\",\"mime_type\":\"image/png\",\"model_used\":\"nano-banana-2\",\"generation_time_ms\":7234}"
      }
    ]
  }
}
```

Key fields in the response:

| Field | Value | Notes |
|---|---|---|
| `image_base64` | Base64-encoded PNG image data | Can be decoded to display the image |
| `mime_type` | `image/png` (typical) | The actual MIME type from Gemini's response |
| `model_used` | `nano-banana-2` | The Nano Banana alias, never the Gemini model ID |
| `generation_time_ms` | `7234` | How long the Gemini API call took |

### Step 9: Claude Code displays the image

Claude Code receives the JSON-RPC response, extracts the `image_base64` field, decodes it from base64, and renders the image inline in the conversation.

You see the generated image of a red apple on a white background.

### Error Flow

If something goes wrong at any step, the error is mapped to a safe code before reaching Claude Code:

| Where it fails | What Claude Code sees | Raw error (hidden) |
|---|---|---|
| Input validation | `"invalid_prompt: prompt is required"` | n/a |
| Gemini content policy | `"content_policy_violation: The prompt was blocked..."` | Raw `genai.APIError{Code: 400}` |
| Gemini quota | `"quota_exceeded: API quota exceeded..."` | Raw `genai.APIError{Code: 429}` |
| Gemini timeout | `"generation_failed: Image generation failed..."` | `context.DeadlineExceeded` |
| Network error | `"generation_failed: Image generation failed..."` | Raw network error text |

Claude Code then shows you a user-friendly error message based on the safe code.
