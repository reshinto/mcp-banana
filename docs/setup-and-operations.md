# Setup and Operations

This guide covers all three deployment modes for mcp-banana. Follow one section top-to-bottom without skipping steps.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Mode 1 — Local (stdio)](#mode-1--local-stdio)
3. [Mode 2 — Docker Dev (HTTP, localhost)](#mode-2--docker-dev-http-localhost)
4. [Mode 3 — Docker Prod (HTTP, public IP, TLS, OAuth)](#mode-3--docker-prod-http-public-ip-tls-oauth)
5. [Environment Variable Reference](#environment-variable-reference)
6. [Per-user Gemini API Keys](#per-user-gemini-api-keys)
7. [Token Generation and Rotation](#token-generation-and-rotation)
8. [Docker Operations](#docker-operations)
9. [Health Check](#health-check)
10. [Makefile Targets](#makefile-targets)
11. [CI Pipeline](#ci-pipeline)
12. [Logs and Monitoring](#logs-and-monitoring)

---

## Prerequisites

### All modes

| Tool | Version | Purpose |
|---|---|---|
| Git | Any | Clone the repository |
| Gemini API key | — | Image generation via Google AI Studio |

Get a Gemini API key at [https://aistudio.google.com/](https://aistudio.google.com/). Sign in, click **Get API Key**, and create a key. The key starts with `AIza`.

### Mode 1 (Local)

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.25 or later | Build the binary |

### Mode 2 and Mode 3 (Docker)

| Tool | Version | Purpose |
|---|---|---|
| Docker | 20.10+ | Container runtime |
| Docker Compose | v2 plugin or v1 standalone | Multi-container orchestration |

### Development tools (optional)

| Tool | Version | Purpose |
|---|---|---|
| golangci-lint | v2.11.4 or later | Code linting |
| OpenSSL | Any | Token generation |

Install golangci-lint:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.11.4
```

---

## Mode 1 — Local (stdio)

Run the binary directly on your machine. Claude Code communicates with it over stdin/stdout. No network port is opened.

**1. Clone the repository.**

```bash
git clone https://github.com/reshinto/mcp-banana.git
cd mcp-banana
```

**2. Create and populate `.env`.**

```bash
cp .env.example .env
```

Open `.env` and set your Gemini API key:

```
GEMINI_API_KEY=AIza...
```

All other variables in `.env` are optional for this mode.

**3. Build and start the server.**

```bash
./scripts/run-local.sh
```

The script loads `.env`, builds the binary with `make build`, and starts it in stdio mode. The server is now waiting for MCP requests on stdin.

**4. Connect Claude Code.**

In a separate terminal (or after the server is registered), run:

```bash
claude mcp add-json --scope user banana \
  '{"command":"./mcp-banana","args":["--transport","stdio"],"env":{"GEMINI_API_KEY":"AIza..."},"type":"stdio"}'
```

Replace `AIza...` with your actual key. The `command` path must be the absolute path to the binary if you call this from a different directory — or navigate to the repo root first.

**5. Test.**

Open a new Claude Code session and ask Claude to generate an image:

```
Generate a photo-realistic image of a red panda sitting on a bamboo branch.
```

Claude routes the request through the MCP tool. A successful response includes the generated image inline.

---

## Mode 2 — Docker Dev (HTTP, localhost)

Run the server in Docker, bound to `127.0.0.1:8847`. Only processes on the same machine can reach it. Use this for local testing of the HTTP transport without exposing anything to the internet.

**1. Clone the repository.**

```bash
git clone https://github.com/reshinto/mcp-banana.git
cd mcp-banana
```

**2. Create and populate `.env`.**

```bash
cp .env.example .env
```

Set the required variables:

```
GEMINI_API_KEY=AIza...
MCP_AUTH_TOKEN=<your-token>
```

Generate a token if you do not have one:

```bash
openssl rand -hex 32
```

**3. Start the server.**

```bash
./scripts/run-docker-dev.sh
```

The script checks for `.env`, then runs `docker compose up -d --build`. The container builds from the local `Dockerfile`, binds to `127.0.0.1:8847`, and restarts automatically on failure.

**4. Verify the server is up.**

```bash
curl http://127.0.0.1:8847/healthz
```

Expected response:

```json
{"status":"ok"}
```

**5. Connect Claude Code.**

```bash
claude mcp add-json --scope user banana \
  '{"type":"http","url":"http://127.0.0.1:8847/mcp","headers":{"Authorization":"Bearer <your-token>"}}'
```

Replace `<your-token>` with the value you set for `MCP_AUTH_TOKEN`.

**6. Test.**

Open a new Claude Code session and ask Claude to generate an image.

---

## Mode 3 — Docker Prod (HTTP, public IP, TLS, OAuth)

Run the server on a public-facing host with TLS termination and optional OAuth 2.1 for Claude Desktop integration. The reference domain in this guide is `mcp.terencekong.net` — substitute your own domain throughout.

### Step 1 — Provision a server

Create an Ubuntu 22.04 LTS server (1 GB RAM minimum) with SSH key authentication. Note the server's public IP address.

Install Docker:

```bash
ssh root@<server-ip>
apt-get update && apt-get install -y docker.io docker-compose-plugin
systemctl enable docker && systemctl start docker
```

Create a deploy user:

```bash
adduser deploy
usermod -aG docker deploy
# Append your SSH public key to /home/deploy/.ssh/authorized_keys
```

### Step 2 — Configure DNS

Create an A record pointing your subdomain to the server's IP:

| Type | Name | Value |
|---|---|---|
| A | `mcp` (or your subdomain) | `<server-ip>` |

This makes `mcp.terencekong.net` resolve to your server. Verify propagation before proceeding:

```bash
dig mcp.terencekong.net
# Should return your server's IP
```

### Step 3 — Obtain a TLS certificate

Use `certbot` with the DNS challenge. This method works without running a web server first:

```bash
certbot certonly --manual --preferred-challenges dns -d mcp.terencekong.net
```

`certbot` prompts you to add a `_acme-challenge` TXT record in your DNS provider. Add the record, wait for propagation, then press Enter. Certificates are written to `/etc/letsencrypt/live/mcp.terencekong.net/`.

Renew certificates before they expire (Let's Encrypt certificates are valid for 90 days):

```bash
certbot renew --manual --preferred-challenges dns
```

### Step 4 — Clone and configure

```bash
sudo git clone https://github.com/reshinto/mcp-banana.git /opt/mcp-banana
sudo chown -R deploy:deploy /opt/mcp-banana
cd /opt/mcp-banana
cp .env.example .env
```

Edit `.env` with the minimum required values for production:

```
GEMINI_API_KEY=AIza...
MCP_AUTH_TOKEN=<your-token>

MCP_TLS_CERT_FILE=/certs/fullchain.pem
MCP_TLS_KEY_FILE=/certs/privkey.pem
```

The `docker-compose.prod.yml` overlay mounts `/etc/letsencrypt/live/mcp.terencekong.net` into the container at `/certs`. The paths above match that mount.

### Step 5 — Configure OAuth (optional)

OAuth enables Claude Desktop to authenticate users through Google, GitHub, or Apple. Skip this step if you only need bearer token auth.

**Register your app with each provider.**

Google — [console.cloud.google.com/apis/credentials](https://console.cloud.google.com/apis/credentials)

Create an OAuth 2.0 Client ID. Set the authorized redirect URI to:
`https://mcp.terencekong.net:8847/callback`

GitHub — [github.com/settings/developers](https://github.com/settings/developers)

Create a new OAuth App. Set the authorization callback URL to:
`https://mcp.terencekong.net:8847/callback`

Apple — [developer.apple.com/account/resources/identifiers](https://developer.apple.com/account/resources/identifiers)

Register a Services ID. Set the return URL to:
`https://mcp.terencekong.net:8847/callback`

**Add credentials to `.env`:**

```
OAUTH_BASE_URL=https://mcp.terencekong.net:8847

OAUTH_GOOGLE_CLIENT_ID=<client-id>
OAUTH_GOOGLE_CLIENT_SECRET=<client-secret>

OAUTH_GITHUB_CLIENT_ID=<client-id>
OAUTH_GITHUB_CLIENT_SECRET=<client-secret>
```

Only providers with both `CLIENT_ID` and `CLIENT_SECRET` set appear on the OAuth login page. You do not need to configure all three.

### Step 6 — Start the server

```bash
./scripts/run-docker-prod.sh
```

The script merges `docker-compose.yml` with `docker-compose.prod.yml` (which overrides the port binding to `0.0.0.0:8847` and mounts the TLS certificate directory), then runs `docker compose up -d --build`.

### Step 7 — Verify

```bash
curl -k https://mcp.terencekong.net:8847/healthz
```

Expected response:

```json
{"status":"ok"}
```

### Step 8 — Connect Claude Code

```bash
claude mcp add-json --scope user banana \
  '{"type":"http","url":"https://mcp.terencekong.net:8847/mcp","headers":{"Authorization":"Bearer <your-token>"}}'
```

### Step 9 — Connect Claude Desktop

Open Claude Desktop, go to **Customize > Connectors**, and add a new connector with URL `https://mcp.terencekong.net:8847/mcp`. If OAuth is configured, Claude Desktop uses the OAuth flow to authenticate.

### Updating production

```bash
ssh deploy@<server-ip>
cd /opt/mcp-banana
git pull origin main
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build --force-recreate
curl -k https://mcp.terencekong.net:8847/healthz
```

Roll back if the new version fails:

```bash
git checkout <previous-commit-sha>
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build --force-recreate
```

---

## Environment Variable Reference

All variables are loaded from `.env` at startup. The server exits immediately with a descriptive error if a required variable is missing or a constraint is violated.

| Variable | Required | Default | Type | Description |
|---|---|---|---|---|
| `GEMINI_API_KEY` | Yes | — | string | Google Gemini API key. Starts with `AIza`. Registered as a secret — redacted from all logs and error output. |
| `MCP_AUTH_TOKEN` | No | — | string | Single bearer token for HTTP authentication. Every HTTP request must include `Authorization: Bearer <token>`. Generate with `openssl rand -hex 32`. Not used in stdio mode. |
| `MCP_AUTH_TOKENS_FILE` | No | — | path | Path to a file containing bearer tokens, one per line. Lines starting with `#` and empty lines are ignored. Hot-reloaded on every request — add or remove tokens without restarting. If both `MCP_AUTH_TOKEN` and `MCP_AUTH_TOKENS_FILE` are set, a request matching either is accepted. |
| `MCP_LOG_LEVEL` | No | `info` | enum | Log verbosity. One of: `debug`, `info`, `warn`, `error`. Logs are JSON-formatted and written to stderr. |
| `MCP_RATE_LIMIT` | No | `30` | int | Maximum requests per minute across all models. Must be a positive integer. |
| `MCP_GLOBAL_CONCURRENCY` | No | `8` | int | Maximum simultaneous in-flight requests across all models. Must be a positive integer. |
| `MCP_PRO_CONCURRENCY` | No | `3` | int | Maximum simultaneous requests for the Pro model (`nano-banana-pro`). Must be a positive integer and must be `<=` `MCP_GLOBAL_CONCURRENCY`. The server exits at startup if this constraint is violated. |
| `MCP_MAX_IMAGE_BYTES` | No | `4194304` | int | Maximum decoded image size in bytes for the `edit_image` tool. Default is 4 MB. Must be a positive integer. |
| `MCP_REQUEST_TIMEOUT_SECS` | No | `120` | int | Per-call timeout for Gemini API requests in seconds. The Pro model can take 15–45 seconds. Set this higher than the slowest expected response. Must be a positive integer. |
| `MCP_TLS_CERT_FILE` | No | — | path | Path to TLS certificate file (PEM format). Both `MCP_TLS_CERT_FILE` and `MCP_TLS_KEY_FILE` must be set together. When set, the server serves HTTPS. |
| `MCP_TLS_KEY_FILE` | No | — | path | Path to TLS private key file (PEM format). Must be set together with `MCP_TLS_CERT_FILE`. |
| `OAUTH_BASE_URL` | No | — | URL | Base URL for OAuth endpoints. Must be HTTPS in production. Example: `https://mcp.terencekong.net:8847`. Required when any OAuth provider is configured. |
| `OAUTH_GOOGLE_CLIENT_ID` | No | — | string | Google OAuth 2.0 client ID. |
| `OAUTH_GOOGLE_CLIENT_SECRET` | No | — | string | Google OAuth 2.0 client secret. Registered as a secret. |
| `OAUTH_GITHUB_CLIENT_ID` | No | — | string | GitHub OAuth App client ID. |
| `OAUTH_GITHUB_CLIENT_SECRET` | No | — | string | GitHub OAuth App client secret. Registered as a secret. |
| `OAUTH_APPLE_CLIENT_ID` | No | — | string | Apple Sign In Services ID. |
| `OAUTH_APPLE_CLIENT_SECRET` | No | — | string | Apple Sign In client secret. Registered as a secret. |

The `.env` file is listed in `.gitignore` and must never be committed.

---

## Per-user Gemini API Keys

HTTP clients can supply their own Gemini API key on a per-request basis. Send the key in the `X-Gemini-API-Key` request header:

```
X-Gemini-API-Key: AIza...
```

The server uses the per-request key for that call and falls back to the server-level `GEMINI_API_KEY` when the header is absent. This lets multiple users share a single deployment while billing against their own Google AI accounts.

Example with curl:

```bash
curl -X POST https://mcp.terencekong.net:8847/mcp \
  -H "Authorization: Bearer <your-token>" \
  -H "X-Gemini-API-Key: AIza..." \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"generate_image","arguments":{"prompt":"a cat"}}}'
```

---

## Token Generation and Rotation

**Generate a new token:**

```bash
openssl rand -hex 32
```

**Rotate the token with guided instructions:**

```bash
make rotate-token
```

This prints a new random token and step-by-step instructions for updating the server's `.env` and your Claude Code configuration.

**Manual rotation steps:**

1. Generate a new token: `openssl rand -hex 32`
2. SSH into the server and update `MCP_AUTH_TOKEN` in `/opt/mcp-banana/.env`
3. Restart the server: `docker compose restart`
4. Update your Claude Code MCP config with the new token:
   ```bash
   claude mcp add-json --scope user banana \
     '{"type":"http","url":"http://127.0.0.1:8847/mcp","headers":{"Authorization":"Bearer <new-token>"}}'
   ```

---

## Docker Operations

All commands below use the base `docker-compose.yml`. For production, append `-f docker-compose.yml -f docker-compose.prod.yml` to every `docker compose` command.

**View logs (follow mode):**

```bash
docker compose logs -f mcp-banana
```

**Stop the server:**

```bash
docker compose down
```

**Restart without rebuilding:**

```bash
docker compose restart
```

**Rebuild and restart:**

```bash
docker compose up -d --build --force-recreate
```

**Check container status:**

```bash
docker compose ps
```

**View resource usage:**

```bash
docker stats mcp-banana
```

**Docker Compose V1 note:** If you see `unknown shorthand flag: 'd' in -d`, your Docker version does not include the Compose V2 plugin. Replace `docker compose` (with a space) with `docker-compose` (with a hyphen) in every command.

---

## Health Check

The `/healthz` endpoint returns HTTP 200 with `{"status":"ok"}` when the server is running.

```bash
# Dev mode (HTTP)
curl http://127.0.0.1:8847/healthz

# Prod mode (HTTPS)
curl https://mcp.terencekong.net:8847/healthz
```

Docker runs an internal health check every 30 seconds. After 3 consecutive failures, Docker marks the container `unhealthy`. Check the status:

```bash
docker compose ps
```

If the container shows `unhealthy`, inspect logs:

```bash
docker compose logs mcp-banana
```

---

## Makefile Targets

| Target | Description |
|---|---|
| `make build` | Compile the binary to `./mcp-banana` with stripped debug info |
| `make test` | Run all tests with race detector and coverage output |
| `make lint` | Run `golangci-lint` |
| `make fmt` | Format all Go source files with `gofmt` |
| `make fmt-check` | Check formatting without modifying files (used in CI) |
| `make vet` | Run `go vet ./...` |
| `make run-stdio` | Build and run in stdio mode |
| `make run-http` | Build and run in HTTP mode on `0.0.0.0:8847` |
| `make clean` | Remove the binary and coverage output |
| `make rotate-token` | Generate a new auth token and print rotation instructions |
| `make quality-gate` | Run the full CI sequence: lint → fmt-check → vet → test |

---

## CI Pipeline

CI runs automatically via GitHub Actions on pushes to `main`, `feat/**`, `fix/**`, `chore/**` branches and on pull requests to `main`.

| Step | Command |
|---|---|
| Lint | `golangci-lint run` |
| Format check | `gofmt -l .` |
| Static analysis | `go vet ./...` |
| Tests | `go test -coverprofile=coverage.out -race ./...` (80% coverage threshold) |
| Build | `go build ./cmd/mcp-banana/` |
| Binary size check | 15 MB limit |
| Docker image build | — |
| Docker image size check | 25 MB limit |

Run the same sequence locally before opening a PR:

```bash
make quality-gate
```

---

## Logs and Monitoring

Logs are written as JSON to stderr and captured by Docker's `json-file` driver (10 MB per file, 3 files rotation):

```bash
docker compose logs -f mcp-banana
```

| Level | When |
|---|---|
| `debug` | Detailed request tracing — development only |
| `info` | Normal startup and request events (default) |
| `warn` | Unexpected but recoverable conditions |
| `error` | Failures requiring attention |

Set `MCP_LOG_LEVEL=debug` temporarily to diagnose issues. Revert to `info` for production.
