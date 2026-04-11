# Setup and Operations

This guide covers all three deployment modes for mcp-banana. Pick one section and follow it top-to-bottom without skipping steps.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Mode 1 — Local (stdio)](#mode-1--local-stdio)
3. [Mode 2 — Docker Dev (HTTP, localhost)](#mode-2--docker-dev-http-localhost)
4. [Mode 3 — Docker Prod (HTTPS, public)](#mode-3--docker-prod-https-public)
5. [Environment Variable Reference](#environment-variable-reference)
6. [Makefile Targets](#makefile-targets)
7. [Docker Operations](#docker-operations)
8. [Health Check](#health-check)

---

## Prerequisites

### All modes

| Tool | Version | Purpose |
|---|---|---|
| Git | Any | Clone the repository |
| Gemini API key | — | Image generation via Google AI Studio |

Get a Gemini API key at [https://aistudio.google.com/](https://aistudio.google.com/). Sign in, click **Get API Key**, and create a key. The key starts with `AIza`.

### Mode 1 only (Local)

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.25 or later | Build the binary |

### Modes 2 and 3 (Docker)

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

All other variables are optional for this mode.

**3. Build and start the server.**

```bash
./scripts/run-local.sh
```

The script loads `.env`, builds the binary with `make build`, and starts it in stdio mode. The server waits for MCP requests on stdin.

**4. Connect Claude Code.**

See [Claude Code Integration](claude-code-integration.md#option-a-local-stdio) for the exact command.

**5. Test.**

Open a new Claude Code session and ask:

```
Generate a photo-realistic image of a red panda sitting on a bamboo branch.
```

A successful response includes the generated image inline.

---

## Mode 2 — Docker Dev (HTTP, localhost)

Run the server in Docker, bound to `127.0.0.1:8847`. Only processes on the same machine can reach it. Use this for local testing of the HTTP transport.

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

See [Claude Code Integration](claude-code-integration.md#option-b-docker-dev-http) for the exact command.

**6. Test.**

Open a new Claude Code session and ask Claude to generate an image.

---

## Mode 3 — Docker Prod (HTTPS, public)

Run the server on a public-facing host with HTTPS and optional OAuth 2.1 for Claude Desktop. This guide uses `mcp.yourdomain.com` as the domain and a DigitalOcean droplet as the server — adapt to your setup.

### Step 1 — Provision a server

Create an Ubuntu 22.04+ server (1 GB RAM minimum) with SSH key authentication. Note the server's **public IP address**.

SSH into the server and run:

```bash
apt-get update
apt-get install -y docker.io docker-compose-plugin certbot
systemctl enable docker && systemctl start docker

# Create a deploy user
adduser deploy && usermod -aG docker deploy

# Open port 8847 (ufw)
sudo ufw allow 8847/tcp
# Or add an inbound TCP 8847 rule in your cloud provider's firewall panel
```

### Step 2 — Configure DNS

You need to create a DNS **A record** that points your subdomain to the server's IP address. This is done in the DNS settings of wherever you manage your domain (e.g., Namecheap, Cloudflare, GoDaddy, Google Domains).

**2.1 Log in to your domain registrar** and go to the DNS management page for your domain.

**2.2 Create an A record:**

| Field | Value | Example |
|---|---|---|
| **Type** | `A` | A |
| **Name** / **Host** | `mcp` (just the subdomain, not the full domain) | `mcp` |
| **Value** / **Points to** | Your server's public IP address | `164.90.xxx.xxx` |
| **TTL** | `300` (or "Automatic") | 300 |

> **What this does:** An A record maps a hostname to an IP address. After this, `mcp.yourdomain.com` will resolve to your server's IP. DNS registrars may call the fields different names — "Host" instead of "Name", or "Points to" instead of "Value".

**2.3 Wait for DNS propagation** (usually 1–5 minutes, can take up to 48 hours):

```bash
dig mcp.yourdomain.com +short
# Should print your server's IP address
```

If `dig` is not installed: `sudo apt-get install -y dnsutils`

If `dig` returns nothing, the record hasn't propagated yet. Wait and try again.

### Step 3 — Obtain a TLS certificate

TLS certificates enable HTTPS. Without them, browsers and Claude Desktop will refuse to connect. We use [Let's Encrypt](https://letsencrypt.org/) to get free certificates.

The DNS challenge method verifies you own the domain by asking you to create a temporary DNS TXT record. This works without needing ports 80 or 443 to be free.

**3.1 Run certbot:**

```bash
sudo certbot certonly --manual --preferred-challenges dns -d mcp.yourdomain.com
```

**3.2 Certbot will display a challenge value** and ask you to create a DNS TXT record:

```
Please deploy a DNS TXT record under the name
_acme-challenge.mcp.yourdomain.com with the following value:

xYz123AbCdEfGhIjKlMnOpQrStUvWxYz1234567890

Before continuing, verify the record is deployed.
```

**3.3 Create the TXT record in your domain registrar.** Go back to your DNS settings and add:

| Field | Value | Example |
|---|---|---|
| **Type** | `TXT` | TXT |
| **Name** / **Host** | `_acme-challenge.mcp` | `_acme-challenge.mcp` |
| **Value** / **Content** | Copy-paste the exact value certbot showed | `xYz123AbCdEfGhIjKlMnOpQrStUvWxYz1234567890` |
| **TTL** | `300` (or "Automatic") | 300 |

> **Important:** The value changes every time you run certbot. Always copy the value from the current certbot output — do not reuse values from previous attempts.

**3.4 Wait 1–2 minutes**, then verify the TXT record propagated before pressing Enter in certbot:

```bash
dig _acme-challenge.mcp.yourdomain.com TXT +short
# Should print the value you just added
```

If `dig` returns nothing, the record hasn't propagated yet. Wait longer before pressing Enter.

**3.5 Press Enter in certbot** after the `dig` command returns your value. Certbot will verify the record and generate your certificates.

**Verify the certificate was created:**

```bash
sudo ls /etc/letsencrypt/live/mcp.yourdomain.com/
# Should show: cert.pem  chain.pem  fullchain.pem  privkey.pem  README
```

> **Note:** `/etc/letsencrypt/` is owned by root with `700` permissions. You must use `sudo` for all commands that access this directory. The `run-docker-prod.sh` script handles this automatically via `sudo test`.

The two files used by mcp-banana are:

- `fullchain.pem` — the TLS certificate
- `privkey.pem` — the private key

The `run-docker-prod.sh` script automatically sets `MCP_TLS_CERT_FILE` and `MCP_TLS_KEY_FILE` in `.env` to point to these files inside the container.

**Verify the certificate is valid:**

```bash
sudo openssl x509 -in /etc/letsencrypt/live/mcp.yourdomain.com/fullchain.pem -noout -dates
```

**Set up auto-renewal** (Let's Encrypt certificates expire after 90 days):

```bash
sudo certbot renew --manual --preferred-challenges dns --dry-run
```

After renewal, restart the container to pick up the new certs:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml restart
```

### Step 4 — Clone and configure

```bash
sudo git clone https://github.com/reshinto/mcp-banana.git /opt/mcp-banana
sudo chown -R deploy:deploy /opt/mcp-banana
cd /opt/mcp-banana
cp .env.example .env
```

**Edit `.env` — complete production configuration:**

```bash
# Domain — used by run-docker-prod.sh to locate TLS certs
MCP_DOMAIN=mcp.yourdomain.com

# TLS (auto-populated by run-docker-prod.sh — leave empty)
MCP_TLS_CERT_FILE=
MCP_TLS_KEY_FILE=

# Bearer token auth (generate with: openssl rand -hex 32)
MCP_AUTH_TOKEN=<paste-your-generated-token>

# Default Gemini key (optional — clients can send X-Gemini-API-Key instead)
GEMINI_API_KEY=

# Server tuning (defaults are fine for most setups)
MCP_LOG_LEVEL=info
MCP_RATE_LIMIT=30
MCP_GLOBAL_CONCURRENCY=8
MCP_PRO_CONCURRENCY=3
MCP_MAX_IMAGE_BYTES=4194304
MCP_REQUEST_TIMEOUT_SECS=120
```

**How `docker-compose.prod.yml` works:** The production overlay mounts the entire `/etc/letsencrypt` directory into the container (read-only). This is necessary because Let's Encrypt stores cert files as symlinks in `live/` pointing to `archive/` — mounting only the `live/` subdirectory would break the symlinks. The `run-docker-prod.sh` script sets `MCP_BIND_ADDRESS=0.0.0.0` (public) and auto-populates the TLS file paths in `.env`.

### Step 5 — Configure OAuth (optional)

Skip this step if you only need bearer token auth or per-user API keys. OAuth enables Claude Desktop to authenticate users through a browser sign-in flow.

Add OAuth credentials to `.env` — see [Authentication](authentication.md#option-3-oauth-21-claude-desktop) for full provider registration steps and the OAuth flow explanation.

```bash
OAUTH_BASE_URL=https://mcp.yourdomain.com:8847

OAUTH_GOOGLE_CLIENT_ID=<client-id>
OAUTH_GOOGLE_CLIENT_SECRET=<client-secret>

OAUTH_GITHUB_CLIENT_ID=<client-id>
OAUTH_GITHUB_CLIENT_SECRET=<client-secret>

# Apple (optional — requires Apple Developer account, see authentication.md)
# OAUTH_APPLE_CLIENT_ID=<services-id>
# OAUTH_APPLE_CLIENT_SECRET=<jwt-secret>
```

Only providers with both `CLIENT_ID` and `CLIENT_SECRET` set appear on the login page.

### Step 6 — Start the server

```bash
./scripts/run-docker-prod.sh
```

The script automatically:

1. Validates `.env` and `MCP_DOMAIN`
2. Detects Docker Compose V1 or V2
3. Auto-populates `OAUTH_BASE_URL`, `MCP_TLS_CERT_FILE`, `MCP_TLS_KEY_FILE` in `.env` from `MCP_DOMAIN`
4. Auto-generates `MCP_AUTH_TOKEN` if empty
5. Installs certbot and generates TLS certificates if missing (interactive DNS challenge)
6. Fixes Let's Encrypt file permissions so the container can read the certs (the container runs as `nonroot`, but Let's Encrypt sets `privkey.pem` to root-only by default)
7. Stops any existing container to free port 8847
8. Builds and starts the Docker container with the production overlay
9. Waits up to 30 seconds for the health check to pass before reporting success

### Step 7 — Verify

**Health check:**

```bash
curl https://mcp.yourdomain.com:8847/healthz
```

Expected: `{"status":"ok"}`

**OAuth metadata (if OAuth is configured):**

```bash
curl https://mcp.yourdomain.com:8847/.well-known/oauth-authorization-server
```

Expected: JSON with `issuer`, `authorization_endpoint`, `token_endpoint`, `registration_endpoint`.

If verification fails, check: DNS propagation, firewall (port 8847 open), TLS cert paths, container logs (`docker compose logs -f`).

### Step 8 — Connect Claude Code

See [Claude Code Integration](claude-code-integration.md#option-c-docker-prod-https) for the exact command.

### Step 9 — Connect Claude Desktop (requires OAuth)

See [Claude Code Integration](claude-code-integration.md#option-d-claude-desktop-oauth) for steps.

### Step 10 — Test

Ask Claude to generate an image:

```
Generate a photo of a sunset over the ocean
```

### Updating production

```bash
ssh deploy@<server-ip>
cd /opt/mcp-banana
git pull origin main
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build --force-recreate
curl https://mcp.yourdomain.com:8847/healthz
```

Roll back if the new version fails:

```bash
git checkout <previous-commit-sha>
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build --force-recreate
```

---

## Environment Variable Reference

All variables are loaded from `.env` at startup. The server exits immediately with a descriptive error if a required variable is missing or invalid.

| Variable | Required | Default | Type | Description |
|---|---|---|---|---|
| `GEMINI_API_KEY` | No | — | string | Google Gemini API key. Starts with `AIza`. If not set, clients must send their own key via `X-Gemini-API-Key` header. |
| `MCP_AUTH_TOKEN` | No | — | string | Single bearer token for HTTP auth. Every HTTP request must include `Authorization: Bearer <token>`. Generate with `openssl rand -hex 32`. Not used in stdio mode. |
| `MCP_AUTH_TOKENS_FILE` | No | — | path | Path to a file containing bearer tokens, one per line. Lines starting with `#` and empty lines are ignored. Hot-reloaded on every request — add or remove tokens without restarting. If both `MCP_AUTH_TOKEN` and `MCP_AUTH_TOKENS_FILE` are set, a request matching either is accepted. |
| `MCP_DOMAIN` | No | — | string | Domain name used by `run-docker-prod.sh` to locate the TLS certificate directory at `/etc/letsencrypt/live/<MCP_DOMAIN>`. Example: `mcp.yourdomain.com`. |
| `MCP_LOG_LEVEL` | No | `info` | enum | Log verbosity. One of: `debug`, `info`, `warn`, `error`. Logs are JSON-formatted and written to stderr. |
| `MCP_RATE_LIMIT` | No | `30` | int | Maximum requests per minute across all models. Must be a positive integer. |
| `MCP_GLOBAL_CONCURRENCY` | No | `8` | int | Maximum simultaneous in-flight requests. Must be a positive integer. |
| `MCP_PRO_CONCURRENCY` | No | `3` | int | Maximum simultaneous requests for `nano-banana-pro`. Must be `<=` `MCP_GLOBAL_CONCURRENCY`. The server exits at startup if this constraint is violated. |
| `MCP_MAX_IMAGE_BYTES` | No | `4194304` | int | Maximum decoded image size in bytes for `edit_image`. Default is 4 MB. |
| `MCP_REQUEST_TIMEOUT_SECS` | No | `120` | int | Per-call timeout for Gemini API requests in seconds. The Pro model can take 15–45 seconds. |
| `MCP_TLS_CERT_FILE` | No | — | path | Path to TLS certificate file (PEM format). Both `MCP_TLS_CERT_FILE` and `MCP_TLS_KEY_FILE` must be set together. When set, the server serves HTTPS. |
| `MCP_TLS_KEY_FILE` | No | — | path | Path to TLS private key file (PEM format). Must be set together with `MCP_TLS_CERT_FILE`. |
| `OAUTH_BASE_URL` | No | — | URL | Base URL for OAuth endpoints. Must be HTTPS in production. Example: `https://mcp.yourdomain.com:8847`. Required when any OAuth provider is configured. |
| `OAUTH_GOOGLE_CLIENT_ID` | No | — | string | Google OAuth 2.0 client ID. |
| `OAUTH_GOOGLE_CLIENT_SECRET` | No | — | string | Google OAuth 2.0 client secret. |
| `OAUTH_GITHUB_CLIENT_ID` | No | — | string | GitHub OAuth App client ID. |
| `OAUTH_GITHUB_CLIENT_SECRET` | No | — | string | GitHub OAuth App client secret. |
| `OAUTH_APPLE_CLIENT_ID` | No | — | string | Apple Sign In Services ID. |
| `OAUTH_APPLE_CLIENT_SECRET` | No | — | string | Apple Sign In client secret (JWT). |

The `.env` file is listed in `.gitignore` and must never be committed.

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

**Log levels:** Set `MCP_LOG_LEVEL=debug` in `.env` temporarily to diagnose issues. Revert to `info` for production. Logs are JSON-formatted on stderr, captured by Docker's `json-file` driver (10 MB per file, 3 files rotation).

**Docker Compose V1 note:** If you see `unknown shorthand flag: 'd' in -d`, replace `docker compose` (with a space) with `docker-compose` (with a hyphen) in every command.

---

## Health Check

The `/healthz` endpoint returns HTTP 200 with `{"status":"ok"}` when the server is running.

```bash
# Dev mode (HTTP)
curl http://127.0.0.1:8847/healthz

# Prod mode (HTTPS)
curl https://mcp.yourdomain.com:8847/healthz
```

Docker runs an internal health check every 30 seconds. After 3 consecutive failures, Docker marks the container `unhealthy`. Check the status:

```bash
docker compose ps
```

If the container shows `unhealthy`, inspect logs:

```bash
docker compose logs mcp-banana
```
