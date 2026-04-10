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

If the container shows startup errors, see [troubleshooting.md](troubleshooting.md) for common problems and fixes.

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

## End-to-End Request Flow

For a detailed walkthrough of the middleware chain, request lifecycle, and security boundaries, see [architecture.md](architecture.md).

**Summary:** Claude Code sends a JSON-RPC `tools/call` request (via stdio or HTTP). In HTTP mode, the request passes through middleware (auth, rate limiting, concurrency, body size). The tool handler validates input via `internal/security/`, calls the Gemini API via `internal/gemini/`, and returns a sanitized result. Errors are mapped to safe codes before reaching Claude Code — see [security.md](security.md) for the error mapping boundary.
