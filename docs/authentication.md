# Authentication

## Overview

Authentication in HTTP mode uses a unified credentials file that maps client identities to Gemini API keys. Two auth methods are available depending on your client.

| Method | When to use | Config needed |
|---|---|---|
| **No auth (SSH tunnel)** | Server reachable only via SSH tunnel | Nothing — auth is skipped |
| **Credentials file** | Claude Code CLI users (bearer tokens or self-registration) | `MCP_CREDENTIALS_FILE` (defaults to `credentials.json`) |
| **OAuth 2.1** | Claude Desktop GUI integration | `OAUTH_BASE_URL` + provider credentials |

The credentials file maps bearer tokens and OAuth identities to Gemini API keys. It is hot-reloaded on every request. If the file does not exist, the server creates it with an empty `{}` object.

## Auth Priority Order

On every request, the middleware checks in this order:

```
Client sends: Authorization: Bearer <token>
  |
  v
1. OAuth access token? Validate via OAuth store. ALLOW if valid and not expired.
2. Credentials file token? Look up token in MCP_CREDENTIALS_FILE. ALLOW if match.
3. Self-registration? Both Authorization: Bearer <token> and X-Gemini-API-Key: <key>
   headers present? Register the token → key mapping in credentials file. ALLOW.
4. No auth configured at all? Skip auth entirely (SSH tunnel mode). ALLOW all requests.
5. No match? 401 {"error":"unauthorized"}
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

Leave `MCP_CREDENTIALS_FILE` at its default. The server creates an empty credentials file if one does not exist. Without any credentials entries, auth is skipped — this is expected for SSH tunnel mode.

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

## Option 2: Credentials File

The credentials file is a JSON object mapping bearer tokens (or OAuth identities) to Gemini API keys. It is hot-reloaded on every request — no server restart needed for changes.

### File format

```json
{
  "a1b2c3d4e5f6...": "AIzaSyA...",
  "f6e5d4c3b2a1...": "AIzaSyB...",
  "google:alice@company.com": "AIzaSyC..."
}
```

Keys are bearer tokens or OAuth identity strings (`provider:email`). Values are Gemini API keys.

### Admin setup

```bash
# Set the credentials file path in .env (defaults to credentials.json)
MCP_CREDENTIALS_FILE=credentials.json
```

The server creates the file with `{}` if it does not exist.

### Adding a user manually (no restart needed)

Generate a bearer token and add it with the user's Gemini API key:

```bash
TOKEN=$(openssl rand -hex 32)
echo "Token for Alice: $TOKEN"
# Edit credentials.json and add: "<token>": "<gemini-api-key>"
```

Share the bearer token with Alice. She adds it to her Claude Code config.

### Self-registration (no admin action needed)

A new user can register themselves by sending both headers on their first request:

- `Authorization: Bearer <token>` — the user generates their own token
- `X-Gemini-API-Key: <gemini-api-key>` — their personal Gemini key

The server writes the token-to-key mapping into the credentials file automatically. Subsequent requests only need the `Authorization` header.

### Revoking a user (no restart needed)

Delete the user's entry from `credentials.json`. Their next request returns `401 {"error":"unauthorized"}` immediately.

### Rotating a token (no restart needed)

1. Generate a new token: `openssl rand -hex 32`
2. Edit `credentials.json`: remove the old token entry, add the new token with the same Gemini key
3. Share the new token with the user — they update their Claude Code config

---

## Option 3: OAuth 2.1 (Claude Desktop)

OAuth 2.1 lets users connect Claude Desktop through a browser sign-in flow. After signing in with a provider, users are prompted to enter their Gemini API key. The server stores the OAuth identity and Gemini key mapping in the credentials file.

| Aspect | Credentials File | OAuth 2.1 |
|---|---|---|
| Client | Claude Code CLI | Claude Desktop GUI |
| Credential management | Manual token or self-registration | Browser-based sign-in + Gemini key prompt |
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

The Gemini API key is required for image generation. Every user provides their own key, which is stored in the credentials file alongside their identity.

### Claude Code (CLI)

Claude Code users provide their Gemini key through one of two methods:

1. **Self-registration (first request):** Send both `Authorization: Bearer <token>` and `X-Gemini-API-Key: <key>` headers. The server writes the mapping to the credentials file. Subsequent requests only need the `Authorization` header.

2. **Admin pre-registration:** An admin adds the token-to-key mapping directly in `credentials.json`.

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "https://mcp.yourdomain.com:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-bearer-token>",
    "X-Gemini-API-Key": "<your-gemini-api-key>"
  }
}'
```

After the first successful request, the `X-Gemini-API-Key` header is no longer needed — the server looks up the key from the credentials file.

### Claude Desktop (GUI)

Claude Desktop users authenticate via OAuth. After signing in with a provider (Google, GitHub, or Apple), the user is prompted to enter their Gemini API key. The server stores the `provider:email` identity and Gemini key mapping in the credentials file.

### Summary

| Client | How Gemini key is provided | Credentials file entry |
|---|---|---|
| **Claude Code** | Self-registration via headers or admin pre-registration | `"bearer_token": "gemini_key"` |
| **Claude Desktop** | Prompted during OAuth sign-in flow | `"provider:email": "gemini_key"` |

### How it works internally

1. The auth middleware identifies the client (OAuth token or bearer token from credentials file).
2. The Gemini API key is looked up from the credentials file using the client's identity.
3. For self-registration, both the bearer token and Gemini key are extracted from headers and written to the credentials file.
4. The key is registered with the output sanitizer — it never appears in responses or logs.
5. The tool handler calls `clientCache.GetClient(ctx, apiKey)`, which returns a cached per-user client or creates a new one.
6. If no key is found for the client identity, the tool returns an error: `"no API key configured"`.

For the Claude Code command that includes these headers, see [Claude Code Integration](claude-code-integration.md).
