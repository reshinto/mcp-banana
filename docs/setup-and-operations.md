# Setup and Operations

## Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.24 or later | Build from source |
| golangci-lint | v2.1.6 or later | Code linting (development) |
| Docker | Any recent version | Container deployment |
| Docker Compose | v2 | Multi-container deployment |
| OpenSSL | Any | Generating auth tokens |
| SSH | Any | Remote deployment access |

## Local Development Setup

### Step 1: Get a Gemini API Key

Visit [https://aistudio.google.com/](https://aistudio.google.com/), sign in with a Google account, and create an API key. The key starts with `AIza`.

### Step 2: Clone and Build

```bash
git clone https://github.com/reshinto/mcp-banana.git
cd mcp-banana
make build
```

This produces a `./mcp-banana` binary.

### Step 3: Set Environment Variables

For local stdio development, only `GEMINI_API_KEY` is required:

```bash
export GEMINI_API_KEY="AIza..."
export MCP_LOG_LEVEL="info"
```

### Step 4: Replace Sentinel Model IDs

Before the server can start, the model IDs in `internal/gemini/registry.go` must be verified against the live Gemini API. The sentinel value `VERIFY_MODEL_ID_BEFORE_RELEASE` triggers a startup failure by design.

See [models.md](models.md) for the verification procedure.

### Step 5: Run in Stdio Mode

```bash
make run-stdio
# or: ./mcp-banana --transport stdio
```

The server starts and waits for JSON-RPC messages on stdin. Claude Code sends tool calls over this channel automatically when configured. See [claude-code-integration.md](claude-code-integration.md) for integration instructions.

### Step 6: Run the Quality Gate

Before committing any changes, run the full CI sequence:

```bash
make quality-gate
```

This runs lint, format check, vet, and tests in order. All steps must pass.

## Configuration Reference

All configuration is loaded from environment variables at startup. Required values must be present; optional values use the listed defaults. A missing required value or a malformed optional value causes the server to exit immediately with a descriptive error.

| Variable | Required | Default | Validation Rules |
|---|---|---|---|
| `GEMINI_API_KEY` | Yes | - | Non-empty string; registered as a secret |
| `MCP_AUTH_TOKEN` | HTTP only | - | Non-empty string when transport is HTTP; registered as a secret |
| `MCP_LOG_LEVEL` | No | `info` | Must be one of: `debug`, `info`, `warn`, `error` |
| `MCP_RATE_LIMIT` | No | `30` | Positive integer; requests per minute |
| `MCP_GLOBAL_CONCURRENCY` | No | `8` | Positive integer; max simultaneous requests |
| `MCP_PRO_CONCURRENCY` | No | `3` | Positive integer; must be <= `MCP_GLOBAL_CONCURRENCY` |
| `MCP_MAX_IMAGE_BYTES` | No | `4194304` | Positive integer; decoded image size limit in bytes (default 4 MB) |
| `MCP_REQUEST_TIMEOUT_SECS` | No | `120` | Positive integer; per-call Gemini API timeout in seconds |

### Relationship Constraint

`MCP_PRO_CONCURRENCY` must be less than or equal to `MCP_GLOBAL_CONCURRENCY`. The server exits at startup if this constraint is violated:

```
MCP_PRO_CONCURRENCY (5) must be <= MCP_GLOBAL_CONCURRENCY (3)
```

### Setting Up the .env File

```bash
cp .env.example .env
# Edit .env with your actual values
```

The `.env` file is listed in `.gitignore` and must never be committed.

## Understanding MCP_AUTH_TOKEN

### What It Is

`MCP_AUTH_TOKEN` is a shared secret (bearer token) that authenticates HTTP clients connecting to the mcp-banana server. It works like a password for the API -- every HTTP request to `/mcp` must include it in the `Authorization` header.

### Why It Is Needed

In **stdio mode** (local development), security relies on OS process isolation -- Claude Code spawns mcp-banana as a child process and communicates over stdin/stdout. No network port is opened, so no authentication is needed.

In **HTTP mode** (remote deployment), the server listens on a network port. Without a bearer token, anyone who can reach that port could call the Gemini API using your API key. The bearer token prevents unauthorized access.

### How to Generate One

The token should be a cryptographically random string. Use OpenSSL:

```bash
openssl rand -hex 32
```

This produces a 64-character hex string like `a1b2c3d4e5f6...`. There is no required format -- any non-empty string works -- but a 32-byte random hex string provides strong security.

### Where to Set It

1. **On the server:** Add it to the `.env` file:
   ```
   MCP_AUTH_TOKEN=a1b2c3d4e5f6...your-generated-token...
   ```

2. **On the client (Claude Code):** Include it in the MCP server configuration:
   ```bash
   claude mcp add-json --scope user banana '{
     "type": "http",
     "url": "http://localhost:8847/mcp",
     "headers": {
       "Authorization": "Bearer a1b2c3d4e5f6...your-generated-token..."
     }
   }'
   ```

The token on the server and client must match exactly. If they do not match, the server returns `401 {"error":"unauthorized"}`.

### Security Properties

- The token is registered with the sanitizer at startup via `security.RegisterSecret()`, so it is automatically redacted from all logs and error output.
- The token is compared using constant-time string comparison in the middleware to prevent timing attacks.
- To rotate the token, run `make rotate-token` which generates a new one and prints update instructions for both server and client.
- The token is never stored in GitHub -- it lives only in the server's `.env` file and in each user's Claude Code config (`~/.claude.json`).

### When It Is Not Needed

- **Stdio transport:** No token required. The server ignores `MCP_AUTH_TOKEN` in stdio mode.
- **Health check endpoint:** `GET /healthz` is exempt from token authentication so Docker health checks and monitoring tools can probe the server without credentials.

---

## How Docker Uses Environment Variables

### The `.env` File

When you run `docker compose up`, Docker Compose reads the `.env` file in the project directory and passes those variables to the container as environment variables. This is configured in `docker-compose.yml`:

```yaml
services:
  mcp-banana:
    env_file:
      - .env
```

The `env_file` directive tells Docker Compose: "read each line of `.env` as `KEY=VALUE` and inject it into the container's environment." Inside the container, the Go application reads them with `os.Getenv("GEMINI_API_KEY")` -- exactly the same way it reads env vars on the host.

### The Flow

```
.env file (on host)
  |
  v
docker compose up (reads .env via env_file directive)
  |
  v
Container environment (KEY=VALUE pairs available to the process)
  |
  v
config.Load() in Go (reads os.Getenv for each variable)
```

### Why This Approach

1. **Secrets stay out of the image.** The `.env` file is on the host filesystem, not baked into the Docker image. The `Dockerfile` does not contain any `ENV` directives for secrets. If someone pulls the Docker image, they do not get your API keys.

2. **Secrets stay out of version control.** The `.env` file is listed in `.gitignore` and `.dockerignore`. Only `.env.example` (with empty values) is committed.

3. **Different environments use different files.** Development and production servers each have their own `.env` with different API keys and tokens. The same Docker image works everywhere -- only the `.env` file changes.

### Important Notes

- The `.env` file must exist on the host before running `docker compose up`. If it is missing, the container starts with empty environment variables and `config.Load()` fails immediately with "GEMINI_API_KEY is required".
- Changes to `.env` require a container restart: `docker compose restart` or `docker compose up -d --force-recreate`.
- You can verify which env vars the container sees with: `docker compose exec mcp-banana env` (but this only works with non-distroless images; the distroless base does not include a shell).

---

## Production Deployment (Docker on DigitalOcean)

### Step 1: Provision a Droplet

Create a DigitalOcean Droplet (Ubuntu 22.04 LTS recommended, 1 GB RAM minimum). Enable SSH key authentication.

### Step 2: Install Docker on the Droplet

```bash
ssh root@<droplet-ip>
apt-get update && apt-get install -y docker.io docker-compose-plugin
systemctl enable docker
systemctl start docker
```

### Step 3: Clone the Repository

```bash
git clone https://github.com/reshinto/mcp-banana.git /opt/mcp-banana
cd /opt/mcp-banana
```

### Step 4: Configure Environment

```bash
cp .env.example .env
nano .env
```

Set at minimum:

```
GEMINI_API_KEY=AIza...
MCP_AUTH_TOKEN=<generate with: openssl rand -hex 32>
```

### Step 5: Verify Model IDs

The model IDs in `internal/gemini/registry.go` must be verified before deploying. The CD pipeline also enforces this with a sentinel check. See [models.md](models.md).

### Step 6: Start the Server

```bash
docker compose up -d --build
```

Docker Compose builds the image locally, starts the container, and binds port 8847 to `127.0.0.1` (loopback only). The container restarts automatically unless explicitly stopped.

### Step 7: Verify Health

```bash
curl http://localhost:8847/healthz
```

Expected response: `{"status":"ok"}`

The container also runs an internal health check every 30 seconds using `mcp-banana --healthcheck`. If 3 consecutive health checks fail, Docker marks the container unhealthy.

## CI/CD Pipeline

![CI/CD Pipeline](diagrams/ci-cd-pipeline.png)

### Continuous Integration

CI runs on:

- Pushes to `feat/**`, `fix/**`, `chore/**` branches
- Pull requests to `main`
- Called automatically by the CD workflow on pushes to `main`

Steps (must all pass before merge):

1. `golangci-lint run` with a 5-minute timeout
2. `gofmt -l .` format check (exits 1 if any file needs reformatting)
3. `go vet ./...` static analysis
4. `go test -coverprofile=coverage.out -race ./... -v` with 80% coverage threshold
5. Build the production binary (`CGO_ENABLED=0`, `-ldflags="-s -w"`, `-trimpath`)
6. Verify binary size is under 15 MB
7. Build the Docker image (build only; does not run - the sentinel model IDs prevent startup)
8. Verify Docker image size is under 25 MB

Coverage is checked with:
```bash
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
awk -v cov="$COVERAGE" 'BEGIN { if (cov < 80.0) { print "Coverage below 80%"; exit 1 } }'
```

### Continuous Deployment

CD runs on pushes to `main` only. It first invokes the CI workflow via `workflow_call`.

The deploy job:

1. Checks out the code and blocks deployment if any sentinel model ID (`VERIFY_MODEL_ID_BEFORE_RELEASE`) is still present in `internal/gemini/registry.go`.
2. SSH-connects to the DigitalOcean droplet using `DEPLOY_HOST`, `DEPLOY_USER`, and `DEPLOY_SSH_KEY` (GitHub environment secrets in the `production` environment).
3. On the droplet: records the current commit SHA, pulls new code, rebuilds and restarts the container.
4. Polls `/healthz` for up to 30 seconds (6 attempts, 5 seconds apart).
5. On success: prunes old Docker images and exits 0.
6. On failure: runs a rollback function that resets to the previous SHA, rebuilds, and polls health again. Warns if rollback also fails.

Concurrent deployments are prevented by a `concurrency: group: deploy-production` key with `cancel-in-progress: false`.

### Deployment Secrets

| Secret | Stored In | Purpose |
|---|---|---|
| `DEPLOY_HOST` | GitHub environment secrets | IP or hostname of the production droplet |
| `DEPLOY_USER` | GitHub environment secrets | SSH username |
| `DEPLOY_SSH_KEY` | GitHub environment secrets | SSH private key for the deployment user |
| `GEMINI_API_KEY` | Server `.env` file | Gemini API authentication |
| `MCP_AUTH_TOKEN` | Server `.env` file | HTTP bearer token |

Application runtime secrets are never stored in GitHub. They live only on the server in `.env`.

## Token Rotation

To rotate the MCP auth token:

```bash
make rotate-token
```

This generates a new random token and prints step-by-step instructions for updating both the server (via SSH) and your Claude Code configuration.

## Monitoring and Health

### Health Endpoint

```bash
curl http://localhost:8847/healthz
```

Returns `{"status":"ok"}` with HTTP 200. Returns an error if the server process is running but unhealthy.

### Logs

Logs are written as JSON to stderr. In Docker, they are captured by the `json-file` driver with a 10 MB cap and 3 file rotation. View with:

```bash
docker compose logs -f mcp-banana
```

Log fields include `time`, `level`, `msg`, and request-specific fields. All secrets are redacted before logging.

### Log Levels

| Level | When to use |
|---|---|
| `debug` | Detailed request tracing (development only) |
| `info` | Normal startup and request events (default) |
| `warn` | Unexpected but recoverable conditions |
| `error` | Failures requiring attention |

Set `MCP_LOG_LEVEL=debug` temporarily to diagnose issues. Revert to `info` for production.

---

## End-to-End Testing Guide

This section walks through testing the mcp-banana server from start to finish, both locally and in Docker.

### Prerequisites for E2E Testing

- The sentinel model IDs in `internal/gemini/registry.go` must be replaced with verified Gemini model IDs. The server refuses to start with sentinel values.
- A valid `GEMINI_API_KEY` from [https://aistudio.google.com/](https://aistudio.google.com/).
- For HTTP mode: an `MCP_AUTH_TOKEN` (generate with `openssl rand -hex 32`).

### Testing Locally (Stdio Mode)

Stdio mode is how Claude Code connects to the server in local development. The server reads JSON-RPC from stdin and writes responses to stdout.

**Step 1: Build and start the server**

```bash
export GEMINI_API_KEY="AIza..."
make build
./mcp-banana --transport stdio
```

The server starts and waits for input. You can now type JSON-RPC requests directly.

**Step 2: Test tool discovery**

Send a `tools/list` JSON-RPC request by pasting this into stdin:

```json
{"jsonrpc":"2.0","id":1,"method":"tools/list"}
```

Expected: a JSON-RPC response listing all 4 tools (`generate_image`, `edit_image`, `list_models`, `recommend_model`) with their schemas.

**Step 3: Test list_models**

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_models"}}
```

Expected: a response with 3 models (nano-banana-2, nano-banana-pro, nano-banana-original). Verify that NO `gemini_id` or `GeminiID` field appears in the output -- this is a security requirement.

**Step 4: Test recommend_model**

```json
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"recommend_model","arguments":{"task_description":"Create a professional product photo","priority":"quality"}}}
```

Expected: a recommendation for `nano-banana-pro` with alternatives.

**Step 5: Test generate_image (requires live Gemini API)**

```json
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"generate_image","arguments":{"prompt":"A red apple on a white background","model":"nano-banana-2"}}}
```

Expected: a response with `image_base64`, `mime_type`, `model_used`, and `generation_time_ms`. The `model_used` field should be `nano-banana-2` (the alias, not a Gemini model ID).

**Step 6: Test error handling**

Send an empty prompt to verify validation:

```json
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"generate_image","arguments":{"prompt":""}}}
```

Expected: an error response containing `invalid_prompt`. The error message should be a safe, predefined string -- not a raw Gemini SDK error.

Press `Ctrl+C` to stop the server.

### Testing Locally (HTTP Mode)

HTTP mode is how remote clients connect. It adds authentication, rate limiting, and concurrency controls.

**Step 1: Start the server in HTTP mode**

```bash
export GEMINI_API_KEY="AIza..."
export MCP_AUTH_TOKEN="test-token-for-local-testing"
make build
./mcp-banana --transport http --addr 127.0.0.1:8847
```

**Step 2: Test health endpoint (no auth required)**

```bash
curl http://127.0.0.1:8847/healthz
```

Expected: `{"status":"ok"}` with HTTP 200.

**Step 3: Test authentication**

```bash
# Missing token -> 401
curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8847/mcp

# Wrong token -> 401
curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer wrong-token" http://127.0.0.1:8847/mcp

# Correct token -> should get a response (may be 400 without a valid JSON-RPC body, but not 401)
curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer test-token-for-local-testing" -X POST http://127.0.0.1:8847/mcp
```

**Step 4: Test tool call over HTTP**

```bash
curl -X POST http://127.0.0.1:8847/mcp \
  -H "Authorization: Bearer test-token-for-local-testing" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_models"}}'
```

Expected: JSON-RPC response with the 3 models.

**Step 5: Test rate limiting**

Send many requests quickly to verify rate limiting kicks in:

```bash
for attempt in $(seq 1 50); do
  curl -s -o /dev/null -w "%{http_code}\n" \
    -X POST http://127.0.0.1:8847/mcp \
    -H "Authorization: Bearer test-token-for-local-testing" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_models"}}';
done
```

Expected: most responses are 200, but after exceeding the rate limit (default 30/min), you should see `429` responses.

### Testing in Docker

Docker testing verifies the full deployment stack: Dockerfile build, environment injection, container health, and network access.

**Step 1: Create the .env file**

```bash
cp .env.example .env
# Edit .env with your values:
# GEMINI_API_KEY=AIza...
# MCP_AUTH_TOKEN=<your-generated-token>
```

**Step 2: Build and start the container**

```bash
docker compose up -d --build
```

Watch the build output for errors. The build has two stages:
1. `golang:1.24-alpine` compiles the binary
2. `gcr.io/distroless/static-debian12:nonroot` runs it

**Step 3: Check container status**

```bash
docker compose ps
```

Expected: the container is `Up` and `healthy`. If it shows `unhealthy` or keeps restarting, check logs:

```bash
docker compose logs mcp-banana
```

Common issues:
- "GEMINI_API_KEY is required" -- the `.env` file is missing or the variable is empty
- "model registry validation failed" -- sentinel model IDs have not been replaced
- "MCP_AUTH_TOKEN is required for HTTP transport" -- the token is not set in `.env`

**Step 4: Test health from the host**

```bash
curl http://127.0.0.1:8847/healthz
```

Expected: `{"status":"ok"}`. This works because `docker-compose.yml` maps host port `127.0.0.1:8847` to container port `8847`.

**Step 5: Test a tool call through Docker**

```bash
curl -X POST http://127.0.0.1:8847/mcp \
  -H "Authorization: Bearer <your-mcp-auth-token>" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_models"}}'
```

Replace `<your-mcp-auth-token>` with the value from your `.env` file.

**Step 6: Test with Claude Code**

After adding the MCP server to Claude Code (see [claude-code-integration.md](claude-code-integration.md)), verify in Claude Code:

1. Ask: "What image generation tools are available?" -- Claude Code should discover all 4 tools.
2. Ask: "List the available Nano Banana models" -- Claude Code should call `list_models`.
3. Ask: "Recommend a model for creating a product photo" -- Claude Code should call `recommend_model`.
4. Ask: "Generate an image of a sunset over mountains" -- Claude Code should call `generate_image` and display the result.

**Step 7: Clean up**

```bash
docker compose down
```

### Unit Tests (Automated)

The unit test suite covers all packages without requiring a live Gemini API key:

```bash
make test
```

This runs all tests with race detection and coverage reporting. Tests use mock implementations of the `GeminiService` interface to simulate API responses.

To run tests for a specific package:

```bash
go test -v ./internal/config/...
go test -v ./internal/gemini/...
go test -v ./internal/security/...
go test -v ./internal/server/...
go test -v ./internal/tools/...
go test -v ./internal/policy/...
```

See [testing.md](testing.md) for the full test inventory and testing patterns.
