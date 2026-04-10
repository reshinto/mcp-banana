# Troubleshooting

## Quick Reference

| Problem | Error / Symptom | Cause | Fix |
|---|---|---|---|
| **Server won't start** | `GEMINI_API_KEY is required` | `GEMINI_API_KEY` not set | Set it in `.env` or your shell; get a key at [aistudio.google.com](https://aistudio.google.com/) |
| **Server won't start** | `registry validation failed: model "..." has unverified GeminiID` | Sentinel model IDs still present in `registry.go` | Follow the verification procedure in [models.md](models.md) |
| **Server won't start** | `both MCP_TLS_CERT_FILE and MCP_TLS_KEY_FILE must be set together` | Only one of the two TLS vars is set | Set both, or clear both |
| **Server won't start** | `MCP_PRO_CONCURRENCY (N) must be <= MCP_GLOBAL_CONCURRENCY (M)` | `MCP_PRO_CONCURRENCY` exceeds the global limit | Lower `MCP_PRO_CONCURRENCY` or raise `MCP_GLOBAL_CONCURRENCY` |
| **Auth failure** | `401 {"error":"unauthorized"}` | Wrong or missing bearer token | Check the `Authorization: Bearer <token>` header matches the server's configured token |
| **Auth failure** | `401` immediately after OAuth sign-in | OAuth access token expired (1-hour TTL) | Re-authenticate via Claude Desktop Connectors to get a fresh token |
| **Auth failure** | No auth configured, requests still rejected | `MCP_AUTH_TOKEN` or `MCP_AUTH_TOKENS_FILE` is set but the token doesn't match | Verify the token in your client config matches exactly what is in `.env` or `tokens.txt` |
| **Connection failure** | `connection refused` on HTTP mode | SSH tunnel not running, or server is down | Start the tunnel; verify the server with `curl http://127.0.0.1:8847/healthz` |
| **Connection failure** | Wrong URL in Claude Code config | Port, host, or path is wrong | URL must be `http[s]://<host>:8847/mcp` — note the `/mcp` path suffix |
| **Connection failure** | Port not exposed in Docker | `ports` block missing from `docker-compose.yml` | Confirm the port mapping is present and `docker compose ps` shows the port bound |
| **Connection failure** | TLS mismatch | Certificate CN/SAN does not match the hostname in the URL | Verify cert covers the exact hostname: `openssl x509 -noout -text -in /certs/fullchain.pem \| grep -A2 SANs` |
| **Image generation failure** | `generation_failed: ...` with quota error | Gemini API quota exceeded | Wait for quota reset, or use a different API key with remaining quota |
| **Image generation failure** | `generation_failed: ...` with content policy error | Prompt or image violated Gemini content policy | Revise the prompt; content filters run on the Gemini side and cannot be bypassed |
| **Image generation failure** | `{"error":"server_busy"}` (HTTP 503) | All `MCP_GLOBAL_CONCURRENCY` slots occupied for 5 seconds | Reduce concurrent calls, or increase `MCP_GLOBAL_CONCURRENCY` in `.env` |
| **Image generation failure** | `{"error":"rate_limited"}` (HTTP 429) | Token bucket exhausted (`MCP_RATE_LIMIT` requests per minute) | Wait for the bucket to refill, or increase `MCP_RATE_LIMIT` in `.env` |
| **Docker: restart loop** | `docker compose ps` shows `Restarting` | Startup error (missing env var, bad config, sentinel IDs) | Run `docker compose logs mcp-banana` to see the specific error message |
| **Docker: health check failing** | Container shows `unhealthy` | Server didn't start, or `/healthz` is unreachable | Run `docker compose logs mcp-banana`; confirm the binary started and is bound to `0.0.0.0:8847` |
| **Docker: cert mount missing** | `tls: failed to find any PEM data` or TLS startup error | Certificate files not mounted into the container | Uncomment the `volumes` block in `docker-compose.yml` and map your cert directory |
| **Claude Code: server not showing** | `banana` absent from `claude mcp list` | Server was not added, or was added with wrong scope | Re-run `claude mcp add-json` with the correct `--scope`; restart Claude Code |
| **Claude Code: wrong config shown** | `claude mcp get banana` shows stdio instead of HTTP | Scope conflict: project-scoped `.mcp.json` shadows user-scoped config | Remove the conflicting entry: `claude mcp remove banana --scope project` |
| **Claude Code: binary not found** | `no such file or directory` or `command not found` on stdio | `mcp-banana` binary not on PATH or wrong path in config | Use the absolute path in `command`, or copy the binary to `/usr/local/bin/` |
| **OAuth: no providers on login page** | `/authorize` renders but shows no sign-in buttons | Both `CLIENT_ID` and `CLIENT_SECRET` are required per provider; at least one is missing | Set both vars for at least one provider in `.env` and restart the server |
| **OAuth: callback error** | `invalid or expired state` after provider redirect | The provider session expired (10-minute window) or the state was tampered | Restart the sign-in flow from scratch in Claude Desktop |
| **OAuth: Claude Desktop can't connect** | Timeout or TLS error in Claude Desktop | Server not reachable over HTTPS, or `OAUTH_BASE_URL` does not match the actual URL | Verify `curl https://banana.yourdomain.com:8847/healthz` succeeds; confirm `OAUTH_BASE_URL` matches exactly |
| **Per-user key: X-Gemini-API-Key not working** | Calls use the server key instead of the personal key | Header not included in Claude Code config | Add `"X-Gemini-API-Key": "<your-key>"` to the `headers` block in your `claude mcp add-json` command |
| **Per-user key: key rejected by Gemini** | `generation_failed` with an auth error | The key in `X-Gemini-API-Key` is invalid or revoked | Test the key: `curl "https://generativelanguage.googleapis.com/v1beta/models?key=<your-key>"` |

---

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
docker compose ps                         # Shows status (Up / unhealthy / Restarting)
docker compose logs mcp-banana            # Last N log lines
docker compose logs -f mcp-banana         # Follow logs in real time
docker compose logs --tail=50 mcp-banana
```

### Test the Gemini API Key Directly

```bash
curl "https://generativelanguage.googleapis.com/v1beta/models?key=$GEMINI_API_KEY"
```

A valid key returns a JSON list of models. An invalid key returns a 400 or 403 error. If this call fails, every image generation request will also fail.

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

### Verify Certificates Inside the Container

```bash
docker compose exec mcp-banana ls /certs
# Should show fullchain.pem and privkey.pem (or your configured filenames)
```

---

## Rate Limiter Behavior

The rate limiter uses a token bucket algorithm. The bucket holds a burst capacity equal to `MCP_RATE_LIMIT` (default 30). Each request consumes one token. The bucket refills at a rate of `MCP_RATE_LIMIT` tokens per minute.

This means you can send up to 30 requests instantly, then the rate drops to 1 request per 2 seconds (30/min). Requests that arrive when the bucket is empty receive HTTP 429 with a `Retry-After` header indicating when to retry.

To increase the burst capacity, raise `MCP_RATE_LIMIT` in `.env` and restart the container.

## Concurrency and Queue Behavior

The global concurrency semaphore limits simultaneous in-flight Gemini API calls to `MCP_GLOBAL_CONCURRENCY` (default 8). An additional semaphore caps pro-model calls at `MCP_PRO_CONCURRENCY` (default 3). `MCP_PRO_CONCURRENCY` must be less than or equal to `MCP_GLOBAL_CONCURRENCY`.

Requests that cannot acquire a semaphore slot within 5 seconds receive HTTP 503 `{"error":"server_busy"}`.

## Graceful Shutdown Behavior

When the container receives `SIGTERM` or `SIGINT` (e.g., from `docker compose stop` or `docker compose down`), the server:

1. Stops accepting new requests immediately.
2. Waits up to 120 seconds for in-flight requests to complete.
3. Exits after all requests finish or after the 120-second timeout, whichever comes first.

The 120-second grace period is configured in `docker-compose.yml` via `stop_grace_period: 120s`. Image generation calls can take up to 45 seconds for pro models, so the grace period is intentionally generous.

If a deployment or restart is taking longer than expected, check for stuck in-flight requests in the logs.
