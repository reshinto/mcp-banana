# Claude Code Integration

## Overview

mcp-banana integrates with Claude Code as an MCP server. Claude Code communicates with the server using the Model Context Protocol over either a local stdio transport or a remote HTTP transport.

| Scope | Use Case | Storage |
|---|---|---|
| **User scope** (`--scope user`) | Personal local development; developer-specific credentials | `~/.claude.json` (not committed) |
| **Project scope** (`--scope project`) | Shared team configuration with no secrets | `.mcp.json` (committed to repository) |

The repository includes a `.mcp.json` with a project-scoped stdio configuration. Each developer supplies their own `GEMINI_API_KEY` via user-scoped config or environment variable.

---

## Option A: Local stdio

Runs `mcp-banana` as a subprocess of Claude Code. Communication happens over stdin/stdout. No network port is opened and no auth token is needed.

**Prerequisites:** Build the binary first with `./scripts/run-local.sh` (or `make build`).

**User-scoped setup (secrets stored locally):**

```bash
claude mcp add-json --scope user banana '{
  "command": "./mcp-banana",
  "args": ["--transport", "stdio"],
  "env": {"GEMINI_API_KEY": "<your-key>"},
  "type": "stdio"
}'
```

Replace `./mcp-banana` with the absolute path to your built binary if you call this from a different directory (e.g., `/usr/local/bin/mcp-banana`). The `env` block is stored only in `~/.claude.json`.

**Project-scoped setup (no secrets, committed to repo):**

```bash
claude mcp add-json --scope project banana '{
  "command": "${MCP_BANANA_BIN:-mcp-banana}",
  "args": ["--transport", "stdio"],
  "type": "stdio"
}'
```

This is saved to `.mcp.json`. Each developer sets `GEMINI_API_KEY` via their own user-scoped config or shell environment. The `${MCP_BANANA_BIN:-mcp-banana}` syntax uses the `MCP_BANANA_BIN` environment variable if set, falling back to `mcp-banana` on PATH.

---

## Option B: Docker Dev (HTTP)

Connect Claude Code to the server running in Docker on localhost. The server must be started first with `./scripts/run-docker-dev.sh`.

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://127.0.0.1:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-mcp-auth-token>",
    "X-Gemini-API-Key": "<your-gemini-api-key>"
  }
}'
```

Replace `<your-mcp-auth-token>` with the value of `MCP_AUTH_TOKEN` from `.env`. The `X-Gemini-API-Key` header is optional — when omitted, the server uses its default `GEMINI_API_KEY`.

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
    "Authorization": "Bearer <your-mcp-auth-token>",
    "X-Gemini-API-Key": "<your-gemini-api-key>"
  }
}'
```

Replace `mcp.yourdomain.com` with your actual domain, `<your-mcp-auth-token>` with the value of `MCP_AUTH_TOKEN` from the server's `.env`, and `<your-gemini-api-key>` with your key from [aistudio.google.com](https://aistudio.google.com/).

Both headers are required in this configuration:

- `Authorization` — authenticates the client with the server
- `X-Gemini-API-Key` — provides your personal Gemini API key (so Gemini charges you, not the server operator)

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

**Sending your own Gemini API key:** Unlike Claude Code, Claude Desktop does not support custom HTTP headers in `mcp-remote` config. If the server has no `GEMINI_API_KEY` configured, ask the server operator to set one, or use Claude Code with Option B or C instead.

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
