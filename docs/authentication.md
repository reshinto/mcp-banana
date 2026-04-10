# Authentication

## Overview

Authentication in HTTP mode is optional. Three approaches are available depending on your security needs.

| Approach | When to use | Config needed |
|---|---|---|
| **SSH tunnel only** (no token) | Server reachable only via SSH tunnel | Nothing -- auth is skipped |
| **Single shared token** | Solo developer or small trusted team | `MCP_AUTH_TOKEN` in `.env` |
| **Per-user tokens file** | Multiple users, individual revocation | `MCP_AUTH_TOKENS_FILE` in `.env` |

If neither `MCP_AUTH_TOKEN` nor `MCP_AUTH_TOKENS_FILE` is set, the server logs a warning and runs without bearer token auth. This is safe when all access goes through an SSH tunnel.

## How Token Auth Works

```
Client sends: Authorization: Bearer <token>
  |
  v
Middleware checks (on every request):
  1. MCP_AUTH_TOKENS_FILE set? Read file from disk (hot-reload), check token. ALLOW if match.
  2. MCP_AUTH_TOKEN set? Check token against env var. ALLOW if match.
  3. Neither set? Skip auth entirely (SSH tunnel mode). ALLOW all requests.
  4. Auth configured but no match? 401 {"error":"unauthorized"}
```

`GET /healthz` is always exempt from token auth so Docker health checks work without credentials.

---

## Option 1: SSH Tunnel Only (No Token)

The server listens only on `127.0.0.1:8847` inside the container and is never exposed to the public internet. Each user creates an SSH tunnel from their local machine, forwarding their local port 8847 to the server's port 8847. The SSH key is the authentication -- no bearer token needed.

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

Leave both `MCP_AUTH_TOKEN` and `MCP_AUTH_TOKENS_FILE` empty in `.env`. The server will log a warning at startup saying auth is disabled -- this is expected.

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

Restart once to pick up the new env var. After that, all token changes are hot-reloaded -- no restart needed.

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
# Share the new token with Alice -- she updates her Claude Code config
```

### Viewing Active Tokens

```bash
# List all active tokens (skip comments and blank lines)
grep -v '^#' /opt/mcp-banana/tokens.txt | grep -v '^$'

# Count active tokens
grep -cv '^#\|^$' /opt/mcp-banana/tokens.txt
```
