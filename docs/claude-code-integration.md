# Claude Code Integration

## Overview

mcp-banana integrates with Claude Code as an MCP server. Claude Code communicates with the server using the Model Context Protocol over either a local stdio transport or a remote HTTP transport.

| Scope | Use Case | Storage |
|---|---|---|
| **User scope** (`--scope user`) | Personal local development; developer-specific credentials | `~/.claude.json` (not committed) |
| **Project scope** (`--scope project`) | Shared team configuration with no secrets | `.mcp.json` (committed to repository) |

The repository includes a `.mcp.json` with a project-scoped stdio configuration. Each developer supplies their own Gemini API key via the credentials file or self-registration headers.

---

## Self-Registration

When connecting over HTTP with a bearer token, send both headers on the first request:

- `Authorization: Bearer <your-bearer-token>` — your identity in the credentials file
- `X-Gemini-API-Key: <your-gemini-api-key>` — your personal Gemini API key

The server writes the token-to-key mapping into the credentials file. After that, the `X-Gemini-API-Key` header is no longer needed — the server looks up the key from the credentials file using the bearer token.

---

## Option A: Local stdio

Runs `mcp-banana` as a subprocess of Claude Code. Communication happens over stdin/stdout. No network port is opened and no auth token is needed.

**Prerequisites:** Build the binary first with `./scripts/run-local.sh` (or `make build`).

**User-scoped setup (secrets stored locally):**

```bash
claude mcp add-json --scope user banana '{
  "command": "./mcp-banana",
  "args": ["--transport", "stdio"],
  "type": "stdio"
}'
```

Replace `./mcp-banana` with the absolute path to your built binary if you call this from a different directory (e.g., `/usr/local/bin/mcp-banana`). The server uses the default `credentials.json` in its working directory to look up Gemini API keys.

**Project-scoped setup (no secrets, committed to repo):**

```bash
claude mcp add-json --scope project banana '{
  "command": "${MCP_BANANA_BIN:-mcp-banana}",
  "args": ["--transport", "stdio"],
  "type": "stdio"
}'
```

This is saved to `.mcp.json`. Each developer's Gemini API key is stored in the credentials file. The `${MCP_BANANA_BIN:-mcp-banana}` syntax uses the `MCP_BANANA_BIN` environment variable if set, falling back to `mcp-banana` on PATH.

---

## Option B: Docker Dev (HTTP)

Connect Claude Code to the server running in Docker on localhost. The server must be started first with `./scripts/run-docker-dev.sh`.

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://127.0.0.1:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-bearer-token>",
    "X-Gemini-API-Key": "<your-gemini-api-key>"
  }
}'
```

On the first request, both headers trigger [self-registration](#self-registration). After that, the `X-Gemini-API-Key` header is no longer needed.

**SSH tunnel setup:** When using an SSH tunnel instead of a bearer token, keep the URL as `http://localhost:8847/mcp` and omit the `Authorization` header:

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://localhost:8847/mcp"
}'
```

See [Authentication — Option 1](authentication.md#option-1-no-auth-ssh-tunnel) for SSH tunnel setup.

---

## Option C: Docker Prod (HTTPS)

Connect Claude Code to a production server with HTTPS. The server must be running with TLS configured (see [Setup and Operations — Mode 3](setup-and-operations.md#mode-3--docker-prod-https-public)).

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

Replace `mcp.yourdomain.com` with your actual domain. Generate your own bearer token with `openssl rand -hex 32` and get your Gemini API key from [aistudio.google.com](https://aistudio.google.com/).

Both headers are needed on the first request for [self-registration](#self-registration). After that, the `X-Gemini-API-Key` header is no longer needed.

---

## Option D: Claude Desktop (via `mcp-remote`)

Connect Claude Desktop to a remote mcp-banana server using `mcp-remote` as a stdio-to-HTTP bridge. This is the recommended approach because Claude Desktop Connectors route through Anthropic's cloud infrastructure, which may not reach non-standard ports like 8847.

`mcp-remote` connects directly from your machine, handles OAuth authentication automatically, and opens a browser for the sign-in flow on first use.

**Prerequisites:**

- Node.js and `npx` installed (`which npx` to verify)
- OAuth configured on the server — see [Authentication — Option 3](authentication.md#option-3-oauth-21-claude-desktop) for provider setup
- TLS enabled on the server (HTTPS is required for OAuth)
- The server is running and reachable at `https://mcp.yourdomain.com:8847`

**Steps:**

1. Open your Claude Desktop config file:

   - **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
   - **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

2. Add the `mcpServers` block (merge with existing config if the file already has content):

   ```json
   {
     "mcpServers": {
       "banana": {
         "command": "npx",
         "args": ["mcp-remote", "https://mcp.yourdomain.com:8847/mcp"]
       }
     }
   }
   ```

   Replace `mcp.yourdomain.com` with your actual domain.

3. Fully quit Claude Desktop (Quit from the menu bar, not just close the window).

4. Reopen Claude Desktop. On the first connection, `mcp-remote` will:
   - Discover the OAuth endpoints via `/.well-known/oauth-protected-resource`
   - Register a client dynamically via `/register`
   - Open your browser to sign in with Google, GitHub, or Apple
   - Store the OAuth tokens locally for future sessions

5. After sign-in, the `banana` tools (generate_image, edit_image, list_models, recommend_model) will appear in Claude Desktop.

**Token refresh:** `mcp-remote` handles token refresh automatically. Access tokens expire after 1 hour; refresh tokens after 30 days. If your refresh token expires, `mcp-remote` will re-open the browser for sign-in.

**Gemini API key:** During the OAuth sign-in flow, users are prompted to enter their Gemini API key. The server stores the key in the credentials file mapped to the user's OAuth identity (`provider:email`).

---

## Verification

After adding the server with any option above:

```bash
claude mcp list
claude mcp get banana
```

Both commands should show the `banana` server entry. If configured correctly, Claude Code will be able to call all four tools.

**Example prompts to test:**

- `List the available image generation models` — calls `list_models`
- `What model do you recommend for a quick draft?` — calls `recommend_model`
- `Generate an image of a sunset over the ocean` — calls `generate_image`
- `Edit this image to make the sky more dramatic` — calls `edit_image` with a provided image

---

## Scope Conflicts

Claude Code resolves MCP servers by name. If the same server name (e.g. `banana`) exists in both project scope (`.mcp.json`) and user scope (`~/.claude.json`), the project-scoped entry takes precedence.

**Common scenario:** You add an HTTP config with `--scope user`, but the repo already has a stdio config in `.mcp.json`. `claude mcp list` shows the HTTP entry as connected, but `claude mcp get banana` shows the project-scoped stdio entry.

**Fix:** Remove the entry from the scope you do not want:

```bash
# Remove the project-scoped stdio entry, keep the user-scoped HTTP entry
claude mcp remove banana --scope project

# Or remove the user-scoped entry, keep the project-scoped one
claude mcp remove banana --scope user
```

After removing the conflicting entry, verify with `claude mcp get banana`.

---

## Troubleshooting

See [Troubleshooting](troubleshooting.md) for common problems including connection failures, auth errors, and binary not found errors.
