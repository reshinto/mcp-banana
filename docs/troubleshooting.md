# Troubleshooting

## Common Problems

| Problem | Error Message | Cause | Fix |
|---|---|---|---|
| Missing API key | `GEMINI_API_KEY is required` | `GEMINI_API_KEY` not set in environment or `.env` | Get a key from [aistudio.google.com](https://aistudio.google.com/) and set it in `.env` or your shell |
| Sentinel model IDs | `registry validation failed: model "..." has unverified GeminiID` | `VERIFY_MODEL_ID_BEFORE_RELEASE` still present in `registry.go` | Follow the verification procedure in [models.md](models.md) |
| Docker container unhealthy | Container shows `unhealthy` in `docker compose ps` | Usually missing `.env`, empty `GEMINI_API_KEY`, or sentinel IDs | Run `docker compose logs mcp-banana` to see the specific startup error |
| Port 8847 already in use | `listen tcp 127.0.0.1:8847: bind: address already in use` | Another process is using the port | Run `lsof -ti:8847 | xargs kill` to kill it, or change the port in `docker-compose.yml` |
| Image generation always fails | `generation_failed: ...` | API key may be invalid or revoked | Test the key directly: `curl "https://generativelanguage.googleapis.com/v1beta/models?key=$GEMINI_API_KEY"` |
| Pro model queue timeout | `{"error":"server_busy"}` (HTTP 503) | All `MCP_PRO_CONCURRENCY` slots are occupied | Lower `MCP_PRO_CONCURRENCY` demand or increase `MCP_GLOBAL_CONCURRENCY` in `.env` |
| Claude Code doesn't discover tools | Server registered but tools are invisible | Server process crashed or model IDs not verified | Run `claude mcp get banana`, check the path and env, then restart Claude Code |
| Wrong Go version | `go: go.mod requires go >= 1.26` | Go version is too old | Install Go 1.26+ from [go.dev/dl](https://go.dev/dl/) |
| `docker compose` not recognized | `unknown shorthand flag: 'd' in -d` | Docker < 20.10 does not include the Compose V2 plugin | Use `docker-compose` (hyphenated) instead, or upgrade Docker to 20.10+ |
| golangci-lint not installed | `golangci-lint: command not found` | Tool not in PATH | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.11.4` |
| Rate limit hit | `{"error":"rate_limited"}` (HTTP 429) | Exceeded `MCP_RATE_LIMIT` requests per minute | Wait for the bucket to refill, or increase `MCP_RATE_LIMIT` in `.env` |

## How to Debug

### Enable Debug Logging

```bash
# Local binary
MCP_LOG_LEVEL=debug ./mcp-banana --transport http --addr 127.0.0.1:8847

# Docker
# Set MCP_LOG_LEVEL=debug in .env, then:
docker compose up -d --force-recreate
docker compose logs -f mcp-banana
```

Debug logs show each request's middleware path, auth result, and Gemini API call duration.

### Check Docker Container State

```bash
docker compose ps                       # Shows status (Up/unhealthy/restarting)
docker compose logs mcp-banana          # Last N log lines
docker compose logs -f mcp-banana       # Follow logs in real time
docker compose logs --tail=50 mcp-banana
```

### Test the Gemini API Key Directly

```bash
curl "https://generativelanguage.googleapis.com/v1beta/models?key=$GEMINI_API_KEY"
```

A valid key returns a JSON list of models. An invalid key returns a 400 or 403 error. If this call fails, the mcp-banana server will also fail on every image generation request.

### Test the Health Endpoint

```bash
curl http://127.0.0.1:8847/healthz
# Expected: {"status":"ok"} with HTTP 200
```

If this fails, the server is not running or not reachable. See [setup-and-operations.md](setup-and-operations.md) for health check and monitoring details.

### Verify Claude Code Can See the Server

```bash
claude mcp list          # Should show "banana" in the list
claude mcp get banana    # Shows the full config entry
```

If `banana` does not appear, re-run the `claude mcp add-json` command from [claude-code-integration.md](claude-code-integration.md). After changes, restart Claude Code completely.

## Rate Limiter Behavior

The rate limiter uses a token bucket algorithm. The bucket holds a burst capacity equal to `MCP_RATE_LIMIT` (default 30). Each request consumes one token. The bucket refills at a rate of `MCP_RATE_LIMIT` tokens per minute.

This means you can send up to 30 requests instantly, then the rate drops to 1 request per 2 seconds (30/min). Requests that arrive when the bucket is empty receive HTTP 429 with a `Retry-After` header indicating when to retry.

To increase the burst capacity, raise `MCP_RATE_LIMIT` in `.env` and restart the container.

## Graceful Shutdown Behavior

When the container receives `SIGTERM` or `SIGINT` (e.g., from `docker compose stop` or `docker compose down`), the server:

1. Stops accepting new requests immediately.
2. Waits up to 120 seconds for in-flight requests to complete.
3. Exits after all requests finish or after the 120-second timeout, whichever comes first.

The 120-second grace period is configured in `docker-compose.yml` via `stop_grace_period: 120s`. Image generation calls can take up to 45 seconds for pro models, so the grace period is intentionally generous.

If a deployment or restart is taking longer than expected, check for stuck in-flight requests in the logs.
