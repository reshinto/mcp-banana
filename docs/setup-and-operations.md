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
