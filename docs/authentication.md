# Authentication

## Overview

Authentication in HTTP mode is optional. Three auth methods and per-user API keys are available depending on your security needs.

| Method | When to use | Config needed |
|---|---|---|
| **No auth (SSH tunnel)** | Server reachable only via SSH tunnel | Nothing — auth is skipped |
| **Bearer token** | Solo developer or small trusted team | `MCP_AUTH_TOKEN` or `MCP_AUTH_TOKENS_FILE` |
| **OAuth 2.1** | Claude Desktop GUI integration | `OAUTH_BASE_URL` + provider credentials |
| **Per-user Gemini keys** | Claude Code users bill their own Gemini quota | `X-Gemini-API-Key` header (Claude Code only — not supported by Claude Desktop) |

If neither `MCP_AUTH_TOKEN` nor `MCP_AUTH_TOKENS_FILE` is set, the server logs a warning and runs without bearer token auth. This is safe when all access goes through an SSH tunnel.

## Auth Priority Order

On every request, the middleware checks in this order:

```
Client sends: Authorization: Bearer <token>
  |
  v
1. MCP_AUTH_TOKENS_FILE set? Read file from disk (hot-reload), check token. ALLOW if match.
2. MCP_AUTH_TOKEN set? Check token against env var. ALLOW if match.
3. OAuth store present? Validate as OAuth access token. ALLOW if valid and not expired.
4. No auth configured at all? Skip auth entirely (SSH tunnel mode). ALLOW all requests.
5. Auth configured but no match? 401 {"error":"unauthorized"}
```

`GET /healthz` is always exempt from auth so Docker health checks work without credentials.

---

## Option 1: No Auth (SSH Tunnel)

The server is accessible only through an SSH tunnel. The SSH key is the authentication — no bearer token is needed.

### Admin Setup (one-time, on the server)

```bash
# Create an SSH user for each team member
adduser alice
adduser bob

# Add each user's SSH public key
mkdir -p /home/alice/.ssh
echo "ssh-ed25519 AAAAC3Nza... alice@company.com" >> /home/alice/.ssh/authorized_keys
chmod 700 /home/alice/.ssh
chmod 600 /home/alice/.ssh/authorized_keys
chown -R alice:alice /home/alice/.ssh
```

Leave both `MCP_AUTH_TOKEN` and `MCP_AUTH_TOKENS_FILE` empty in `.env`. The server will log a warning at startup — this is expected.

### User Setup

```bash
# Open the SSH tunnel (keep this terminal running)
ssh -N -L 8847:127.0.0.1:8847 alice@<droplet-ip>
```

For a persistent tunnel that auto-reconnects:

```bash
brew install autossh
autossh -M 0 -N -L 8847:127.0.0.1:8847 alice@<droplet-ip>
```

Or use an SSH config entry:

```
Host mcp-banana-tunnel
    HostName <droplet-ip>
    User alice
    LocalForward 8847 127.0.0.1:8847
    ServerAliveInterval 30
    ServerAliveCountMax 3
```

Then connect with: `ssh -N mcp-banana-tunnel`

See [Claude Code Integration](claude-code-integration.md#option-b-docker-dev-http) for the Claude Code command — use `http://localhost:8847/mcp` without an `Authorization` header.

### Revoking a User

```bash
# Remove the user entirely
userdel -r alice

# Or just remove their SSH key
sed -i '/alice@company.com/d' /home/alice/.ssh/authorized_keys
```

Their tunnel disconnects immediately and they cannot reconnect.

---

## Option 2: Bearer Token

### Single Token (solo or small team)

Generate a token:

```bash
openssl rand -hex 32
```

Add to server `.env`:

```
MCP_AUTH_TOKEN=<generated-token>
```

Restart the container (single token is read at startup):

```bash
docker compose restart
```

**Limitations:** Token rotation requires every user to update their Claude Code config. No per-user revocation.

### Tokens File (recommended for teams)

Each user gets a unique token stored in a file that is re-read on every request. Add, remove, or rotate tokens without restarting the server.

**Admin setup:**

```bash
# Create the tokens file
touch /opt/mcp-banana/tokens.txt
chmod 600 /opt/mcp-banana/tokens.txt

# Generate a token for each user (lines starting with # are comments)
echo "# Alice (alice@company.com) - generated $(date +%Y-%m-%d)" >> /opt/mcp-banana/tokens.txt
openssl rand -hex 32 >> /opt/mcp-banana/tokens.txt

echo "# Bob (bob@company.com) - generated $(date +%Y-%m-%d)" >> /opt/mcp-banana/tokens.txt
openssl rand -hex 32 >> /opt/mcp-banana/tokens.txt
```

File format:

```
# Alice (alice@company.com) - generated 2026-04-10
a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2
# Bob (bob@company.com) - generated 2026-04-10
f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3b2a1f6e5d4c3b2a1f6e5
```

Add to `.env`:

```
MCP_AUTH_TOKENS_FILE=/opt/mcp-banana/tokens.txt
```

Restart once to pick up the new env var. After that, all token changes are hot-reloaded — no restart needed.

**Adding a user (no restart needed):**

```bash
echo "# Charlie (charlie@company.com) - generated $(date +%Y-%m-%d)" >> /opt/mcp-banana/tokens.txt
openssl rand -hex 32 >> /opt/mcp-banana/tokens.txt
```

**Revoking a user (no restart needed):**

Delete the user's comment and token lines from `tokens.txt`. Their next request returns `401 {"error":"unauthorized"}` immediately.

**Rotating a token (no restart needed):**

```bash
NEW_TOKEN=$(openssl rand -hex 32)
echo "New token for Alice: $NEW_TOKEN"
# Edit tokens.txt and replace Alice's old token line with the new one
nano /opt/mcp-banana/tokens.txt
# Share the new token with Alice — she updates her Claude Code config
```

**Manual token rotation steps:**

1. Generate a new token: `openssl rand -hex 32`
2. SSH into the server and update the token in `/opt/mcp-banana/.env` or `tokens.txt`
3. Restart if using `MCP_AUTH_TOKEN`: `docker compose restart`
4. Update your Claude Code MCP config with the new token (see [Claude Code Integration](claude-code-integration.md))

Alternatively, run `make rotate-token` for guided instructions.

**Viewing active tokens:**

```bash
# List all active tokens (skip comments and blank lines)
grep -v '^#' /opt/mcp-banana/tokens.txt | grep -v '^$'

# Count active tokens
grep -cv '^#\|^$' /opt/mcp-banana/tokens.txt
```

---

## Option 3: OAuth 2.1 (Claude Desktop)

OAuth 2.1 lets users connect Claude Desktop through a browser sign-in flow instead of manually configuring a bearer token. The server acts as an OAuth authorization server, delegating identity verification to Google, GitHub, or Apple.

| Aspect | Bearer Token | OAuth 2.1 |
|---|---|---|
| Client | Claude Code CLI | Claude Desktop GUI |
| Credential management | Manual token distribution | Browser-based sign-in |
| Transport | HTTP or SSH tunnel | HTTPS only |
| Token lifetime | Until rotated | 1 hour (access), 30 days (refresh) |
| PKCE required | No | Yes (S256 only) |

### Prerequisites

- A public subdomain (e.g., `mcp.yourdomain.com`) pointing to your server
- A TLS certificate for that subdomain — see [Setup and Operations](setup-and-operations.md#step-3--obtain-a-tls-certificate) for certbot instructions
- OAuth credentials from at least one provider

### Provider Setup

**Google** — [console.cloud.google.com/apis/credentials](https://console.cloud.google.com/apis/credentials)

1. Create a new OAuth 2.0 Client ID (type: Web application)
2. Add authorized redirect URI: `https://mcp.yourdomain.com:8847/callback`
3. Copy the client ID and secret into `.env`:

```
OAUTH_GOOGLE_CLIENT_ID=<your-client-id>
OAUTH_GOOGLE_CLIENT_SECRET=<your-client-secret>
```

**GitHub** — [github.com/settings/developers](https://github.com/settings/developers)

1. Create a new OAuth App
2. Set authorization callback URL: `https://mcp.yourdomain.com:8847/callback`
3. Copy the client ID and secret into `.env`:

```
OAUTH_GITHUB_CLIENT_ID=<your-client-id>
OAUTH_GITHUB_CLIENT_SECRET=<your-client-secret>
```

**Apple** — [developer.apple.com/account/resources/identifiers](https://developer.apple.com/account/resources/identifiers)

1. Create a Services ID
2. Set return URL: `https://mcp.yourdomain.com:8847/callback`
3. Create a private key (Keys section) with Sign In with Apple enabled
4. Generate a JWT client secret from the private key (valid up to 6 months)
5. Copy the IDs into `.env`:

```
OAUTH_APPLE_CLIENT_ID=<services-id>
OAUTH_APPLE_CLIENT_SECRET=<jwt-secret>
```

> **Apple limitations:** Requires an Apple Developer account ($99/year), HTTPS with a registered domain (no localhost), and a JWT-based client secret that expires every 6 months. You must regenerate it before it expires and update `OAUTH_APPLE_CLIENT_SECRET` in `.env`.

Also set `OAUTH_BASE_URL` (no trailing slash):

```
OAUTH_BASE_URL=https://mcp.yourdomain.com:8847
```

Only providers with both `CLIENT_ID` and `CLIENT_SECRET` set appear on the login page.

### How the OAuth Flow Works

1. Claude Desktop fetches `/.well-known/oauth-authorization-server` to discover endpoints.
2. Claude Desktop sends a dynamic registration request to `/register`. The server issues a `client_id`.
3. Claude Desktop redirects the user to `/authorize` with a PKCE code challenge (`S256` only). The server renders the provider login page.
4. The user clicks a provider button. The server redirects to the provider's authorization endpoint.
5. The provider authenticates the user and redirects back to `/callback` with an authorization code.
6. The server issues its own MCP authorization code (10-minute TTL) and redirects Claude Desktop to its registered redirect URI.
7. Claude Desktop POSTs to `/token` with the MCP authorization code and PKCE verifier. The server issues an access token (1-hour TTL) and a refresh token (30-day TTL).
8. Claude Desktop includes the access token in every subsequent MCP request as `Authorization: Bearer <access-token>`.
9. When the access token expires, Claude Desktop uses the refresh token to get a new token pair. Refresh tokens are single-use.

### Token TTLs

| Token type | Lifetime | Notes |
|---|---|---|
| Authorization code | 10 minutes | Single-use; consumed on exchange |
| Provider session (state) | 10 minutes | Single-use; consumed on callback |
| Access token | 1 hour | Multi-use |
| Refresh token | 30 days | Single-use; replaced on each refresh |

### Local OAuth Testing

Test the OAuth flow on your local machine before deploying to production.

| Provider | `http://localhost` callbacks | Notes |
|---|---|---|
| Google | Yes | Add `http://localhost:8847/callback` as an authorized redirect URI |
| GitHub | Yes | Set callback URL to `http://localhost:8847/callback` |
| Apple | No | Requires HTTPS with a registered domain. Test Apple only on production. |

**Steps:**

1. Register a dev OAuth app with Google or GitHub using `http://localhost:8847/callback` as the redirect URI.

2. Add credentials to `.env`:

```
OAUTH_BASE_URL=http://localhost:8847
OAUTH_GOOGLE_CLIENT_ID=<your-dev-client-id>
OAUTH_GOOGLE_CLIENT_SECRET=<your-dev-client-secret>
```

3. Restart the server:

```bash
docker compose down && docker compose up -d
```

4. Verify the metadata endpoint:

```bash
curl -s http://localhost:8847/.well-known/oauth-authorization-server | python3 -m json.tool
```

5. Register a test client:

```bash
curl -s -X POST http://localhost:8847/register \
  -H "Content-Type: application/json" \
  -d '{"client_name":"test","redirect_uris":["http://localhost:3000/cb"]}' | python3 -m json.tool
```

Copy the `client_id` from the response.

6. Open the login page in a browser:

```
http://localhost:8847/authorize?response_type=code&client_id=<client-id>&redirect_uri=http://localhost:3000/cb&state=test123&code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&code_challenge_method=S256
```

You should see the login page with a "Sign in with Google" button (or whichever providers you configured).

---

## Gemini API Key: Claude Code vs Claude Desktop

The Gemini API key is required for image generation. How it's provided depends on which client you use.

### Claude Code (CLI)

Claude Code lets you set custom HTTP headers in your MCP config. You can provide your Gemini API key per-user via the `X-Gemini-API-Key` header:

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "https://mcp.yourdomain.com:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-mcp-auth-token>",
    "X-Gemini-API-Key": "<your-gemini-api-key>"
  }
}'
```

Each Claude Code user sends their own key. The server does not need `GEMINI_API_KEY` set in `.env` — the per-request header is sufficient.

### Claude Desktop (GUI)

Claude Desktop uses OAuth to authenticate. It controls the HTTP requests and **does not support custom headers** like `X-Gemini-API-Key`. This means Claude Desktop users cannot send their own Gemini key.

For Claude Desktop, you **must** set `GEMINI_API_KEY` in the server's `.env` file. All Claude Desktop users share this server-side key.

```bash
# In .env on the production server
GEMINI_API_KEY=AIza...
```

### Summary

| Client | How Gemini key is provided | Server `GEMINI_API_KEY` needed? |
|---|---|---|
| **Claude Code** | Per-user via `X-Gemini-API-Key` header | No — each user sends their own |
| **Claude Desktop** | Server-side default in `.env` | **Yes** — all users share it |
| **Both clients** | Header overrides server default | Yes — Claude Desktop needs it as fallback |

If you support both Claude Code and Claude Desktop users, set `GEMINI_API_KEY` in `.env` for Claude Desktop, and Claude Code users can optionally override it with their own key via the header.

### How it works internally

1. The HTTP middleware extracts `X-Gemini-API-Key` from the request header (if present).
2. The key is registered with the output sanitizer — it never appears in responses or logs.
3. The key is stored in the request context using a package-private context key.
4. The tool handler calls `clientCache.GetClient(ctx, apiKey)`, which returns a cached per-user client or creates a new one.
5. If no per-request key is provided, the default server-level client (from `GEMINI_API_KEY`) is used.
6. If neither is available, the tool returns an error: `"no API key configured"`.

For the Claude Code command that includes this header, see [Claude Code Integration](claude-code-integration.md).
