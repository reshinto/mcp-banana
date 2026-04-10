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

## Option A: Local Stdio Mode (Recommended for Development)

Stdio mode runs `mcp-banana` as a subprocess of Claude Code. Communication happens over stdin/stdout. No network port is opened and no auth token is needed.

**Prerequisites:** Build the binary (`make build`), obtain a Gemini API key, and verify model IDs in `internal/gemini/registry.go` (see [models.md](models.md)).

### User-Scoped Setup (Secrets Stored Locally)

```bash
claude mcp add-json --scope user banana '{
  "command": "/usr/local/bin/mcp-banana",
  "args": ["--transport", "stdio"],
  "env": {
    "GEMINI_API_KEY": "<your-gemini-api-key>"
  },
  "type": "stdio"
}'
```

Replace `/usr/local/bin/mcp-banana` with the actual path to your built binary. The `env` block is stored only in `~/.claude.json` and is not visible to other team members.

### Project-Scoped Setup (No Secrets, Committed to Repo)

```bash
claude mcp add-json --scope project banana '{
  "command": "${MCP_BANANA_BIN:-mcp-banana}",
  "args": ["--transport", "stdio"],
  "type": "stdio"
}'
```

This configuration is saved to `.mcp.json` and is already committed in this repository. Each developer sets `GEMINI_API_KEY` via their own user-scoped config or shell environment. The `${MCP_BANANA_BIN:-mcp-banana}` syntax uses the `MCP_BANANA_BIN` environment variable if set, falling back to `mcp-banana` on `PATH`.

## Option B: Remote HTTP Mode (For a Deployed Server)

HTTP mode connects Claude Code to a running mcp-banana server over the network. Every request requires a bearer token in the `Authorization` header.

For auth setup details (SSH tunnel, single token, or per-user tokens), see [authentication.md](authentication.md).

### Basic HTTP Connection

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://<server-ip>:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-mcp-auth-token>"
  }
}'
```

### SSH Tunnel (Recommended for Production)

The server binds to `127.0.0.1:8847` inside Docker (not publicly exposed). Open a tunnel first:

```bash
ssh -N -L 8847:127.0.0.1:8847 <user>@<droplet-ip>
```

Then connect using `localhost`:

```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://localhost:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-mcp-auth-token>"
  }
}'
```

## Verification

After adding the server:

```bash
claude mcp list
claude mcp get banana
```

Both commands should show the `banana` server entry. If configured correctly and model IDs are verified, Claude Code will be able to call all four tools.

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---|---|---|
| `claude mcp list` does not show `banana` | Server was not added | Re-run `claude mcp add-json` with the correct `--scope` |
| `GEMINI_API_KEY is required` error on startup | API key not set | Add the `env` block to your user-scoped config or set the variable in your shell |
| `registry validation failed: model "..." has unverified GeminiID` | Sentinel model IDs still present | Follow the verification procedure in [models.md](models.md) |
| HTTP 401 Unauthorized | Wrong or missing auth token | Verify the token in your Claude Code config matches the server |
| `Connection refused` on HTTP mode | SSH tunnel not running or server is down | Start the tunnel or check the server with `curl http://localhost:8847/healthz` |
| Server starts but tools return errors | Gemini API issue or quota exceeded | Check `docker compose logs -f mcp-banana` on the remote server |
| Binary not found | `mcp-banana` not on PATH | Use the full absolute path in `command`, or copy the binary to `/usr/local/bin/` |

See [troubleshooting.md](troubleshooting.md) for additional common issues and debugging steps.

## Verifying Tool Access

Once integrated, you can ask Claude Code to call the tools directly:

- "List the available image generation models" (calls `list_models`)
- "What model do you recommend for a quick draft?" (calls `recommend_model`)
- "Generate an image of a sunset over the ocean" (calls `generate_image`)
- "Edit this image to make the sky more dramatic" (calls `edit_image` with a provided image)
