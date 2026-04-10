# Authentication

## Overview

Authentication in HTTP mode is optional. Four approaches are available depending on your security needs.

| Approach | When to use | Config needed |
|---|---|---|
| **No auth** | Server reachable only via SSH tunnel | Nothing — auth is skipped |
| **Single shared token** | Solo developer or small trusted team | `MCP_AUTH_TOKEN` in `.env` |
| **Per-user tokens file** | Multiple users, individual revocation | `MCP_AUTH_TOKENS_FILE` in `.env` |
| **OAuth 2.1** | Claude Desktop GUI integration | `OAUTH_BASE_URL` + provider credentials in `.env` |

If neither `MCP_AUTH_TOKEN` nor `MCP_AUTH_TOKENS_FILE` is set, the server logs a warning and runs without bearer token auth. This is safe when all access goes through an SSH tunnel.

## How Token Auth Works

```
Client sends: Authorization: Bearer <token>
  |
  v
Middleware checks (on every request):
  1. MCP_AUTH_TOKENS_FILE set? Read file from disk (hot-reload), check token. ALLOW if match.
  2. MCP_AUTH_TOKEN set? Check token against env var. ALLOW if match.
  3. OAuth store present? Validate as OAuth access token. ALLOW if valid and not expired.
  4. Neither static auth configured? Skip auth entirely (SSH tunnel mode). ALLOW all requests.
  5. Auth configured but no match? 401 {"error":"unauthorized"}
```

`GET /healthz` is always exempt from auth so Docker health checks work without credentials.

---

## Option 1: SSH Tunnel Only (No Token)

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

Configure Claude Code with no `Authorization` header:

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://localhost:8847/mcp"
}'
```

### Revoking a User

```bash
# Remove the user entirely
userdel -r alice

# Or just remove their SSH key
sed -i '/alice@company.com/d' /home/alice/.ssh/authorized_keys
```

Their tunnel disconnects immediately and they cannot reconnect.

---

## Option 2: Single Shared Token

For a solo developer or small trusted team where SSH tunnel setup is not practical.

### Admin Setup

```bash
# Generate a token
openssl rand -hex 32

# Add to server .env
MCP_AUTH_TOKEN=<generated-token>

# Restart the container (single token is read at startup)
docker compose restart
```

### User Setup

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://<server-ip-or-tunnel>:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-token>"
  }
}'
```

### Limitations

- Token rotation requires every user to update their Claude Code config.
- Revoking the token removes access for all users simultaneously.
- No per-user revocation is possible.

For teams, use Option 3 instead.

---

## Option 3: Per-User Tokens File (Recommended for Teams)

Each user gets a unique token stored in a file that is re-read on every request. Add, remove, or rotate tokens without restarting the server.

### Admin Setup

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

The file format:

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

### User Setup

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://<server-ip-or-tunnel>:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-token>"
  }
}'
```

### Adding a User (no restart needed)

```bash
echo "# Charlie (charlie@company.com) - generated $(date +%Y-%m-%d)" >> /opt/mcp-banana/tokens.txt
openssl rand -hex 32 >> /opt/mcp-banana/tokens.txt
```

The server picks up the new token on the next request.

### Revoking a User (no restart needed)

Delete the user's comment and token lines from `tokens.txt`. Their next request returns `401 {"error":"unauthorized"}` immediately.

### Rotating a Token (no restart needed)

```bash
NEW_TOKEN=$(openssl rand -hex 32)
echo "New token for Alice: $NEW_TOKEN"
# Edit tokens.txt and replace Alice's old token line with the new one
nano /opt/mcp-banana/tokens.txt
# Share the new token with Alice — she updates her Claude Code config
```

### Viewing Active Tokens

```bash
# List all active tokens (skip comments and blank lines)
grep -v '^#' /opt/mcp-banana/tokens.txt | grep -v '^$'

# Count active tokens
grep -cv '^#\|^$' /opt/mcp-banana/tokens.txt
```

---

## Option 4: OAuth 2.1 (Claude Desktop)

OAuth 2.1 enables Claude Desktop users to authenticate via a browser sign-in flow instead of manually configuring a bearer token. The server acts as an OAuth authorization server, delegating identity verification to a third-party provider (Google, GitHub, or Apple).

### How It Differs from Bearer Token Auth

| Aspect | Bearer Token | OAuth 2.1 |
|---|---|---|
| Client | Claude Code CLI | Claude Desktop GUI |
| Credential management | Manual token distribution | Browser-based sign-in |
| Transport | HTTP or SSH tunnel | HTTPS only |
| Token lifetime | Until rotated | 1 hour (access), 30 days (refresh) |
| PKCE required | No | Yes (S256 only) |

### Prerequisites

- A public subdomain (e.g., `banana.yourdomain.com`) pointing to your server
- A TLS certificate for that subdomain (see [setup-and-operations.md](setup-and-operations.md) for `certbot` instructions)
- OAuth credentials from at least one provider

### Provider Setup

**Google**

1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials) and create an OAuth 2.0 Client ID.
2. Set the authorized redirect URI to: `https://banana.yourdomain.com:8847/callback`
3. Copy the client ID and secret into `.env`:

```
OAUTH_GOOGLE_CLIENT_ID=<your-client-id>
OAUTH_GOOGLE_CLIENT_SECRET=<your-client-secret>
```

**GitHub**

1. Go to [GitHub Developer Settings](https://github.com/settings/developers) and create a new OAuth App.
2. Set the authorization callback URL to: `https://banana.yourdomain.com:8847/callback`
3. Copy the client ID and secret into `.env`:

```
OAUTH_GITHUB_CLIENT_ID=<your-client-id>
OAUTH_GITHUB_CLIENT_SECRET=<your-client-secret>
```

**Apple**

1. Go to [Apple Developer — Identifiers](https://developer.apple.com/account/resources/identifiers) and register a Services ID.
2. Set the return URL to: `https://banana.yourdomain.com:8847/callback`
3. Copy the client ID and secret into `.env`:

```
OAUTH_APPLE_CLIENT_ID=<your-services-id>
OAUTH_APPLE_CLIENT_SECRET=<your-secret-key>
```

### Server Configuration

Set `OAUTH_BASE_URL` to the public HTTPS base URL of your server (no trailing slash):

```
OAUTH_BASE_URL=https://banana.yourdomain.com:8847
```

Mount the TLS certificate and key files, and point the server at them:

```
MCP_TLS_CERT_FILE=/certs/fullchain.pem
MCP_TLS_KEY_FILE=/certs/privkey.pem
```

Both `MCP_TLS_CERT_FILE` and `MCP_TLS_KEY_FILE` must be set together or both left empty — setting only one is a startup error.

Restart the server. Only providers with both `CLIENT_ID` and `CLIENT_SECRET` set appear on the login page (`/authorize`).

### OAuth Flow

1. Claude Desktop fetches `/.well-known/oauth-authorization-server` to discover endpoints.
2. Claude Desktop sends a dynamic registration request to `/register` (RFC 7591). The server issues a `client_id`.
3. Claude Desktop redirects the user to `/authorize` with a PKCE code challenge (method must be `S256`). The server renders the provider login page.
4. The user clicks a provider button. The server redirects to the provider's authorization endpoint.
5. The provider authenticates the user and redirects back to `/callback` with an authorization code.
6. The server validates the provider code, issues its own MCP authorization code (10-minute TTL), and redirects Claude Desktop back to its registered redirect URI.
7. Claude Desktop POSTs to `/token` with the MCP authorization code and PKCE verifier. The server verifies the PKCE S256 challenge and issues an access token (1-hour TTL) and a refresh token (30-day TTL).
8. Claude Desktop includes the access token in every subsequent MCP request as `Authorization: Bearer <access-token>`.
9. When the access token expires, Claude Desktop uses the refresh token to obtain a new token pair. Refresh tokens are single-use — each refresh issues a new refresh token.

---

## Per-User Gemini API Keys

Any authenticated request can supply a personal Gemini API key via the `X-Gemini-API-Key` header. When present, the server uses that key for the Gemini API call instead of the server's `GEMINI_API_KEY`. The per-request key is registered with the output sanitizer so it is never echoed back in responses or logs.

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://<server-ip-or-tunnel>:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-mcp-auth-token>",
    "X-Gemini-API-Key": "<your-personal-gemini-key>"
  }
}'
```

This is useful in multi-user deployments where each developer has their own Gemini quota. The `X-Gemini-API-Key` header works with all three auth options above.
