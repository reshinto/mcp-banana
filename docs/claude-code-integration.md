# Claude Code Integration

## Overview

mcp-banana integrates with Claude Code as an MCP server. Claude Code communicates with the server using the Model Context Protocol over either a local stdio transport or a remote HTTP transport.

## Team Adoption

| Scope | Use Case | Storage |
|---|---|---|
| **User scope** (`--scope user`) | Personal local development; developer-specific credentials | `~/.claude.json` (not committed) |
| **Project scope** (`--scope project`) | Shared team configuration with no secrets | `.mcp.json` (committed to repository) |
| **HTTP** | Preferred transport for remote/shared server | Direct HTTP or SSH-tunneled HTTP |
| **SSE** | Deprecated | Do not use |

The repository includes a `.mcp.json` with a project-scoped stdio configuration. Each developer supplies their own `GEMINI_API_KEY` via user-scoped config or environment variable.

---

## Option A: Local Stdio Mode (Development)

Stdio mode runs `mcp-banana` as a subprocess of Claude Code. Communication happens over stdin/stdout. No network port is opened and no auth token is needed.

**Prerequisites:** Build the binary (`make build`), obtain a Gemini API key, and verify model IDs in `internal/gemini/registry.go` (see [models.md](models.md)).

### User-Scoped Setup (Secrets Stored Locally)

```bash
claude mcp add-json --scope user banana '{
  "command": "./mcp-banana",
  "args": ["--transport", "stdio"],
  "env": {"GEMINI_API_KEY": "<your-key>"},
  "type": "stdio"
}'
```

Replace `./mcp-banana` with the absolute path to your built binary if it is not in the current directory (e.g., `/usr/local/bin/mcp-banana`). The `env` block is stored only in `~/.claude.json` and is not visible to other team members.

### Project-Scoped Setup (No Secrets, Committed to Repo)

```bash
claude mcp add-json --scope project banana '{
  "command": "${MCP_BANANA_BIN:-mcp-banana}",
  "args": ["--transport", "stdio"],
  "type": "stdio"
}'
```

This configuration is saved to `.mcp.json` and is already committed in this repository. Each developer sets `GEMINI_API_KEY` via their own user-scoped config or shell environment. The `${MCP_BANANA_BIN:-mcp-banana}` syntax uses the `MCP_BANANA_BIN` environment variable if set, falling back to `mcp-banana` on `PATH`.

---

## Option B: HTTP Mode (Docker Dev or Remote)

HTTP mode connects Claude Code to a running mcp-banana server over the network. Every request requires a bearer token in the `Authorization` header (unless the server is configured for no-auth SSH tunnel mode — see [authentication.md](authentication.md)).

### Basic HTTP Connection

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://127.0.0.1:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-token>"
  }
}'
```

Replace `127.0.0.1:8847` with the actual server address. When using an SSH tunnel, keep `127.0.0.1:8847` as the URL and let the tunnel handle routing (see [authentication.md — Option 1: SSH Tunnel](authentication.md) for setup instructions).

### HTTP Connection with Per-User Gemini Key

If you want tool calls to use your personal Gemini API key instead of the server's shared key, add the `X-Gemini-API-Key` header:

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://127.0.0.1:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-token>",
    "X-Gemini-API-Key": "<your-personal-gemini-key>"
  }
}'
```

The server registers the per-request key with the output sanitizer — it is never echoed back in tool responses or logs.

---

## Option C: Claude Desktop GUI (OAuth)

OAuth 2.1 lets users connect Claude Desktop to a remote mcp-banana server through a browser-based sign-in flow. No manual token distribution is needed.

**Prerequisites:**

- A public subdomain (e.g., `banana.yourdomain.com`) with an A record pointing to your server's IP
- A TLS certificate for that subdomain (see [setup-and-operations.md](setup-and-operations.md) for `certbot` instructions)
- OAuth credentials from at least one provider (Google, GitHub, or Apple) — see [authentication.md — Option 4: OAuth 2.1](authentication.md) for provider registration steps
- `OAUTH_BASE_URL`, provider credentials, `MCP_TLS_CERT_FILE`, and `MCP_TLS_KEY_FILE` set in `.env`

**Setup:**

1. Configure OAuth and TLS in `.env` on your server:

```bash
OAUTH_BASE_URL=https://banana.yourdomain.com:8847
OAUTH_GOOGLE_CLIENT_ID=<your-client-id>
OAUTH_GOOGLE_CLIENT_SECRET=<your-client-secret>
MCP_TLS_CERT_FILE=/certs/fullchain.pem
MCP_TLS_KEY_FILE=/certs/privkey.pem
```

2. Uncomment the `volumes` block in `docker-compose.yml` to mount your TLS certificates:

```yaml
volumes:
  - /etc/letsencrypt/live/banana.yourdomain.com:/certs:ro
```

3. Restart the server:

```bash
docker compose up -d --force-recreate
```

4. Verify the HTTPS endpoint is reachable:

```bash
curl https://banana.yourdomain.com:8847/healthz
# Expected: {"status":"ok"}
```

5. In Claude Desktop, open **Customize > Connectors** and add the server URL:

```
https://banana.yourdomain.com:8847/mcp
```

6. Claude Desktop fetches the OAuth discovery document at `/.well-known/oauth-authorization-server`, registers a client dynamically, and opens the login page at `/authorize`. Click your provider and complete the browser sign-in flow.

After sign-in, Claude Desktop stores the OAuth access and refresh tokens and handles token refresh automatically (access tokens expire after 1 hour; refresh tokens after 30 days). You can start using the image generation tools immediately.

---

## Verification

After adding the server with any option above:

```bash
claude mcp list
claude mcp get banana
```

Both commands should show the `banana` server entry. If configured correctly and model IDs are verified, Claude Code will be able to call all four tools.

### Example Prompts

Once integrated, you can call the tools directly by asking Claude:

- "List the available image generation models" — calls `list_models`
- "What model do you recommend for a quick draft?" — calls `recommend_model`
- "Generate an image of a sunset over the ocean" — calls `generate_image`
- "Edit this image to make the sky more dramatic" — calls `edit_image` with a provided image

---

## Scope Conflicts

Claude Code resolves MCP servers by name. If the same server name (e.g. `banana`) exists in both project scope (`.mcp.json`) and user scope (`~/.claude.json`), the project-scoped entry takes precedence. This means `claude mcp get banana` and `claude mcp list` may show different results — `list` checks connectivity across all scopes, while `get` returns the highest-priority match.

**Common scenario:** You add an HTTP config with `--scope user`, but the repo already has a stdio config in `.mcp.json`. `claude mcp list` shows the HTTP entry as connected, but `claude mcp get banana` shows the project-scoped stdio entry (which may be failing).

**Fix:** Remove the entry from the scope you do not want:

```bash
# Remove the project-scoped stdio entry, keep the user-scoped HTTP entry
claude mcp remove banana --scope project

# Or remove the user-scoped entry, keep the project-scoped one
claude mcp remove banana --scope user
```

After removing the conflicting entry, verify with `claude mcp get banana`.
