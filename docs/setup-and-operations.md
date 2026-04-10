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
| `MCP_AUTH_TOKEN` | No | - | Single bearer token for HTTP auth; registered as a secret |
| `MCP_AUTH_TOKENS_FILE` | No | - | Path to a file with one token per line; hot-reloaded on every request |
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

## Understanding Authentication (Bearer Tokens)

### Overview

Authentication in HTTP mode is **optional**. There are three approaches, depending on your security needs:

| Approach | When to use | Config needed |
|---|---|---|
| **SSH tunnel only** (no token) | Server is only reachable via SSH tunnel | Nothing -- auth is skipped |
| **Single shared token** | Solo developer or small trusted team | `MCP_AUTH_TOKEN` in `.env` |
| **Per-user tokens file** | Multiple users, individual revocation | `MCP_AUTH_TOKENS_FILE` in `.env` |

If neither `MCP_AUTH_TOKEN` nor `MCP_AUTH_TOKENS_FILE` is set, the server logs a warning and runs without bearer token auth. This is safe when all access goes through an SSH tunnel.

### Option 1: SSH Tunnel Only (No Token)

This is the simplest and most secure approach. The mcp-banana server listens only on `127.0.0.1:8847` inside the droplet (configured in `docker-compose.yml`). It is never exposed to the public internet. Each user creates an SSH tunnel from their local machine to the droplet, which forwards their local port 8847 to the server's port 8847 inside the droplet.

No bearer token is needed -- the SSH key itself is the authentication. Leave both `MCP_AUTH_TOKEN` and `MCP_AUTH_TOKENS_FILE` empty (or remove them) in `.env`.

#### Admin Setup (one-time, on the DigitalOcean droplet)

**Step 1: Create an SSH user account for each team member**

```bash
# SSH to the droplet as root
ssh root@<droplet-ip>

# Create a user account for Alice
adduser alice
# (set a password or skip -- SSH key auth is preferred)

# Create a user account for Bob
adduser bob
```

**Step 2: Add each user's SSH public key**

Each team member generates an SSH key pair on their local machine (if they don't already have one):

```bash
# On the team member's local machine:
ssh-keygen -t ed25519 -C "alice@company.com"
# Accept defaults, or choose a custom path
# This creates ~/.ssh/id_ed25519 (private) and ~/.ssh/id_ed25519.pub (public)
```

The team member sends you their **public key** (the `.pub` file). Never share the private key.

On the droplet, add their public key:

```bash
# As root on the droplet:
mkdir -p /home/alice/.ssh
echo "ssh-ed25519 AAAAC3Nza... alice@company.com" >> /home/alice/.ssh/authorized_keys
chmod 700 /home/alice/.ssh
chmod 600 /home/alice/.ssh/authorized_keys
chown -R alice:alice /home/alice/.ssh
```

Repeat for each team member.

**Step 3: Verify `.env` has no token configured**

```bash
# On the droplet:
cat /opt/mcp-banana/.env | grep -E "MCP_AUTH_TOKEN|MCP_AUTH_TOKENS_FILE"
```

Both should be empty or absent. The server will log a warning at startup saying auth is disabled -- this is expected in SSH-tunnel mode.

#### User Setup (each team member, on their local machine)

**Step 1: Open the SSH tunnel**

```bash
ssh -N -L 8847:127.0.0.1:8847 alice@<droplet-ip>
```

What this does:
- `-N` -- don't open a shell, just forward the port
- `-L 8847:127.0.0.1:8847` -- forward local port 8847 to the droplet's localhost port 8847
- `alice@<droplet-ip>` -- authenticate as `alice` using her SSH key

This command blocks (keeps the tunnel open). Open a new terminal for Claude Code.

**Step 2: Keep the tunnel running (optional but recommended)**

The basic `ssh -N` command disconnects if your network drops. Use `autossh` for a persistent tunnel:

```bash
# Install autossh (macOS)
brew install autossh

# Persistent tunnel that auto-reconnects
autossh -M 0 -N -L 8847:127.0.0.1:8847 alice@<droplet-ip>
```

Or add it to your SSH config (`~/.ssh/config`) for convenience:

```
Host mcp-banana-tunnel
    HostName <droplet-ip>
    User alice
    LocalForward 8847 127.0.0.1:8847
    ServerAliveInterval 30
    ServerAliveCountMax 3
```

Then connect with just:

```bash
ssh -N mcp-banana-tunnel
```

**Step 3: Configure Claude Code**

With the tunnel running, `localhost:8847` on your machine reaches the mcp-banana server. Configure Claude Code:

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://localhost:8847/mcp"
}'
```

No `Authorization` header needed -- the tunnel provides authentication.

**Step 4: Verify the connection**

```bash
# Test the tunnel is working
curl http://localhost:8847/healthz
# Expected: {"status":"ok"}

# Verify Claude Code sees the server
claude mcp list
claude mcp get banana
```

#### Revoking a User's Access

Remove their SSH user account or delete their key from `authorized_keys`:

```bash
# On the droplet:
# Option A: Delete the user entirely
userdel -r alice

# Option B: Just remove their SSH key
sed -i '/alice@company.com/d' /home/alice/.ssh/authorized_keys
```

Their tunnel will disconnect immediately, and they cannot reconnect.

---

### Option 2: Single Shared Token

For a solo developer or small trusted team where SSH tunnel setup is not practical.

#### Admin Setup

**Step 1: Generate a token**

```bash
openssl rand -hex 32
```

This produces a 64-character random hex string, e.g.: `e4a1c3b2d5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2`

**Step 2: Add to server `.env`**

```bash
# On the droplet:
nano /opt/mcp-banana/.env
```

Set:

```
MCP_AUTH_TOKEN=e4a1c3b2d5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2
```

Restart the container (single token from env var is read at startup):

```bash
docker compose restart
```

**Step 3: Give the token to each user**

Share the token securely (not via unencrypted email or Slack). Each user puts it in their Claude Code config.

#### User Setup

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://<server-ip-or-tunnel>:8847/mcp",
  "headers": {
    "Authorization": "Bearer e4a1c3b2d5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2"
  }
}'
```

#### Limitations

- If you rotate this token, **every user** must update their Claude Code config.
- If you revoke it, **everyone** loses access simultaneously.
- There is no way to revoke one user without affecting others.

For teams, use Option 3 instead.

---

### Option 3: Per-User Tokens File (Recommended for Teams)

Each user gets their own unique token. Tokens are stored in a file on the server that is re-read on every request -- you can add, remove, or rotate tokens without restarting the server or Docker container.

#### Admin Setup

**Step 1: Create the tokens file**

```bash
# On the droplet:
touch /opt/mcp-banana/tokens.txt
chmod 600 /opt/mcp-banana/tokens.txt
```

**Step 2: Generate a token for each user**

```bash
# Generate Alice's token
echo "# Alice (alice@company.com) - generated $(date +%Y-%m-%d)" >> /opt/mcp-banana/tokens.txt
openssl rand -hex 32 >> /opt/mcp-banana/tokens.txt

# Generate Bob's token
echo "# Bob (bob@company.com) - generated $(date +%Y-%m-%d)" >> /opt/mcp-banana/tokens.txt
openssl rand -hex 32 >> /opt/mcp-banana/tokens.txt
```

The file will look like:

```
# Alice (alice@company.com) - generated 2026-04-10
a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2
# Bob (bob@company.com) - generated 2026-04-10
f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3b2a1f6e5
```

Lines starting with `#` are comments (ignored). Empty lines are also ignored. Everything else is a valid token.

**Step 3: Tell the server where to find the file**

Add to `.env`:

```
MCP_AUTH_TOKENS_FILE=/opt/mcp-banana/tokens.txt
```

If the server is already running, restart it once to pick up the new env var:

```bash
docker compose restart
```

After this initial restart, all future token changes are hot-reloaded (no restart needed).

**Step 4: Share each user's token securely**

Send Alice her token via a secure channel. She puts it in her Claude Code config (see User Setup below).

#### User Setup

You receive a token from the admin (e.g., `a1b2c3d4e5f6...`). Configure Claude Code:

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://<server-ip-or-tunnel>:8847/mcp",
  "headers": {
    "Authorization": "Bearer a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
  }
}'
```

Verify:

```bash
claude mcp list
claude mcp get banana
```

#### Admin: Adding a New User

No restart needed:

```bash
# On the droplet:
echo "# Charlie (charlie@company.com) - generated $(date +%Y-%m-%d)" >> /opt/mcp-banana/tokens.txt
openssl rand -hex 32 >> /opt/mcp-banana/tokens.txt
```

Note the generated token and share it with Charlie. The server picks it up on the next request automatically.

#### Admin: Revoking a User

No restart needed:

```bash
# On the droplet:
# Remove Alice's comment and token lines
nano /opt/mcp-banana/tokens.txt
# Delete the two lines (comment + token) for Alice, then save
```

Alice's next request will immediately return `401 {"error":"unauthorized"}`.

#### Admin: Rotating a User's Token

No restart needed:

```bash
# On the droplet:
# 1. Generate a new token
NEW_TOKEN=$(openssl rand -hex 32)
echo "New token for Alice: $NEW_TOKEN"

# 2. Replace Alice's old token in the file
# Edit tokens.txt and swap Alice's token line with the new one
nano /opt/mcp-banana/tokens.txt

# 3. Share the new token with Alice -- she updates her Claude Code config
```

#### Admin: Viewing Active Tokens

```bash
# On the droplet:
# Show all active tokens (skip comments and blank lines)
grep -v '^#' /opt/mcp-banana/tokens.txt | grep -v '^$'

# Count active tokens
grep -cv '^#\|^$' /opt/mcp-banana/tokens.txt
```

---

### How Token Auth Works

```
Client (Claude Code)
  sends: Authorization: Bearer <token>
    |
    v
Middleware checks (on every request):
  1. Is MCP_AUTH_TOKENS_FILE set?
     -> Read the file from disk (hot-reload)
     -> Check if <token> matches any line in the file
     -> If match: ALLOW
  2. Is MCP_AUTH_TOKEN set?
     -> Check if <token> matches the env var value
     -> If match: ALLOW
  3. Neither set?
     -> Skip auth entirely (SSH tunnel mode)
     -> ALLOW all requests
  4. Auth configured but no match?
     -> 401 {"error":"unauthorized"}
```

### Security Properties

- Tokens from `MCP_AUTH_TOKEN` are registered with the sanitizer at startup, so they are redacted from logs.
- `GET /healthz` is always exempt from token auth so Docker health checks work without credentials.
- The tokens file is re-read from disk on every request. This makes hot-reload possible but means the file must be readable by the process running inside the Docker container.
- Error responses never include the token value, whether auth succeeds or fails.
- To rotate tokens: update the tokens file (no restart needed), then share the new token with the affected user.

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

### Step 3: Create a Deploy User (recommended)

Running as root is not recommended. Create a deploy user with Docker access:

```bash
# On the droplet as root:
adduser deploy
usermod -aG docker deploy
```

Add your SSH public key to `/home/deploy/.ssh/authorized_keys`.

### Step 4: Clone the Repository

```bash
# As the deploy user:
sudo git clone https://github.com/reshinto/mcp-banana.git /opt/mcp-banana
sudo chown -R deploy:deploy /opt/mcp-banana
cd /opt/mcp-banana
```

Doing this manually ensures you can verify the setup before running Docker.

### Step 5: Configure Environment

```bash
cp .env.example .env
nano .env
```

Set at minimum:

```
GEMINI_API_KEY=AIza...
MCP_AUTH_TOKEN=<generate with: openssl rand -hex 32>
```

The `.env` file is read by Docker Compose via the `env_file: .env` directive in `docker-compose.yml`. See [How Docker Uses Environment Variables](#how-docker-uses-environment-variables) above for details on how this works.

The `.env` file is NOT tracked by git (listed in `.gitignore`), so `git pull` or `git reset --hard` will never overwrite it.

### Step 6: Verify Model IDs

The model IDs in `internal/gemini/registry.go` must be verified before deploying. The CD pipeline also enforces this with a sentinel check. See [models.md](models.md).

### Step 7: Build and Start the Server

```bash
docker compose up -d --build
```

What this does:
1. **`docker compose`** reads `docker-compose.yml` which defines the `mcp-banana` service
2. **`--build`** triggers a Docker image build using the `Dockerfile`:
   - Stage 1 (`golang:1.24-alpine`): downloads Go dependencies, compiles the binary
   - Stage 2 (`gcr.io/distroless/static-debian12:nonroot`): copies only the compiled binary into a minimal image
3. **`-d`** runs the container in detached mode (background)
4. The container starts with `CMD ["--transport", "http", "--addr", "0.0.0.0:8847"]`
5. Docker Compose maps host port `127.0.0.1:8847` to container port `8847` (localhost only)
6. Docker Compose injects all variables from `.env` into the container environment via `env_file: .env`

The container restarts automatically on failure (`restart: unless-stopped`) with a 120-second graceful shutdown period (`stop_grace_period: 120s`) and a 768 MB memory limit (`mem_limit: 768m`).

### Step 8: Verify Health

```bash
curl http://localhost:8847/healthz
```

Expected response: `{"status":"ok"}`

The container also runs an internal health check every 30 seconds using `mcp-banana --healthcheck`. If 3 consecutive health checks fail, Docker marks the container unhealthy. Check container status:

```bash
docker compose ps
```

If the container shows as `unhealthy`, check logs:

```bash
docker compose logs mcp-banana
```

Common startup failures:
- `GEMINI_API_KEY is required` -- `.env` is missing or `GEMINI_API_KEY` is empty
- `model registry validation failed` -- sentinel IDs not replaced (see Step 6)
- `MCP_AUTH_TOKEN is required for HTTP transport mode` -- `MCP_AUTH_TOKEN` not set in `.env`

## CI Pipeline

![CI Pipeline](diagrams/ci-cd-pipeline.png)

CI runs automatically via GitHub Actions (`.github/workflows/ci.yml`) on:

- Pushes to `main`, `feat/**`, `fix/**`, `chore/**` branches
- Pull requests to `main`

Steps (must all pass before merge):

1. `golangci-lint run` with a 5-minute timeout
2. `gofmt -l .` format check (exits 1 if any file needs reformatting)
3. `go vet ./...` static analysis
4. `go test -coverprofile=coverage.out -race ./... -v` with 80% coverage threshold
5. Build the production binary (`CGO_ENABLED=0`, `-ldflags="-s -w"`, `-trimpath`)
6. Verify binary size is under 15 MB
7. Build the Docker image (build only; does not run -- the sentinel model IDs prevent startup)
8. Verify Docker image size is under 25 MB

### Manual Deployment to Production

Deployment to DigitalOcean is done manually via SSH. There is no automated CD pipeline -- you control when production updates happen.

```bash
# SSH to the droplet
ssh deploy@<droplet-ip>
cd /opt/mcp-banana

# Pull latest code
git pull origin main

# Rebuild and restart the container
docker compose up -d --build --force-recreate

# Verify health
curl http://127.0.0.1:8847/healthz
```

If the new version fails, roll back:

```bash
git checkout <previous-commit-sha>
docker compose up -d --build --force-recreate
```

### Secrets

| Secret | Stored In | Purpose |
|---|---|---|
| `GEMINI_API_KEY` | Server `.env` file | Gemini API authentication |
| `MCP_AUTH_TOKEN` | Server `.env` file (optional) | Single bearer token for HTTP auth |
| `MCP_AUTH_TOKENS_FILE` | Server `.env` file (optional) | Path to per-user tokens file |

All secrets live only on the server. They are never stored in GitHub or version control.

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
