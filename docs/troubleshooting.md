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
| **Docker: cert mount missing** | `tls: failed to find any PEM data` or TLS startup error | Certificate files not mounted into the container | Ensure `docker-compose.prod.yml` mounts the cert directory and `MCP_TLS_CERT_FILE`/`MCP_TLS_KEY_FILE` are set in `.env`. See [TLS certificate generation](#tls-certificate-generation) below |
| **TLS cert directory not found** | `WARNING: TLS certificate directory not found at /etc/letsencrypt/live/<domain>` when running `run-docker-prod.sh` | TLS certificates have not been generated yet, or you are running the prod script on a local machine that doesn't have certs | If on the production server, generate certs with certbot (see [TLS certificate generation](#tls-certificate-generation) below). If on your local machine, this warning is expected — use `./scripts/run-docker-dev.sh` for local development instead |
| **Claude Code: server not showing** | `banana` absent from `claude mcp list` | Server was not added, or was added with wrong scope | Re-run `claude mcp add-json` with the correct `--scope`; restart Claude Code |
| **Claude Code: wrong config shown** | `claude mcp get banana` shows stdio instead of HTTP | Scope conflict: project-scoped `.mcp.json` shadows user-scoped config | Remove the conflicting entry: `claude mcp remove banana --scope project` |
| **Claude Code: binary not found** | `no such file or directory` or `command not found` on stdio | `mcp-banana` binary not on PATH or wrong path in config | Use the absolute path in `command`, or copy the binary to `/usr/local/bin/` |
| **OAuth: no providers on login page** | `/authorize` renders but shows no sign-in buttons | Both `CLIENT_ID` and `CLIENT_SECRET` are required per provider; at least one is missing | Set both vars for at least one provider in `.env` and restart the server |
| **OAuth: callback error** | `invalid or expired state` after provider redirect | The provider session expired (10-minute window) or the state was tampered | Restart the sign-in flow from scratch in Claude Desktop |
| **OAuth: Claude Desktop can't connect** | Timeout or TLS error in Claude Desktop | Server not reachable over HTTPS, or `OAUTH_BASE_URL` does not match the actual URL | Verify `curl https://mcp.yourdomain.com:8847/healthz` succeeds; confirm `OAUTH_BASE_URL` matches exactly |
| **OAuth: Apple callback fails locally** | Apple redirect fails or returns error | Apple Sign-In does not support `http://localhost` callbacks | Test Apple only on production with HTTPS. Use Google or GitHub for local OAuth testing |
| **OAuth: Apple secret expired** | `invalid_client` from Apple after weeks/months | Apple uses JWT-based client secrets that expire (max 6 months) | Regenerate the JWT from your Apple private key and update `OAUTH_APPLE_CLIENT_SECRET` in `.env` |
| **OAuth: 401 triggers OAuth flow** | Claude Code shows "authenticate banana MCP server" instead of calling tools | An `Authorization` header with an invalid/mismatched token causes 401, which Claude Code interprets as needing OAuth | Remove the `Authorization` header from your Claude Code MCP config if auth is disabled, or set the correct token |
| **Per-user key: X-Gemini-API-Key not working** | Calls use the server key instead of the personal key | Header not included in Claude Code config | Add `"X-Gemini-API-Key": "<your-key>"` to the `headers` block in your `claude mcp add-json` command |
| **Per-user key: key rejected by Gemini** | `generation_failed` with an auth error | The key in `X-Gemini-API-Key` is invalid or revoked | Test the key: `curl "https://generativelanguage.googleapis.com/v1beta/models?key=<your-key>"` |

---

## TLS Certificate Generation

TLS certificates are required for production HTTPS. Without them, the server cannot serve HTTPS and Claude Desktop OAuth will not work. mcp-banana uses [Let's Encrypt](https://letsencrypt.org/) for free, auto-renewable certificates.

### Prerequisites

- A domain name (e.g., `mcp.yourdomain.com`) with a DNS A record pointing to your server
- SSH access to the production server
- `certbot` installed on the server

### Step-by-step

**1. Install certbot on your server:**

```bash
sudo apt-get update && sudo apt-get install -y certbot
```

**2. Run certbot with the DNS challenge:**

This method does not require ports 80 or 443 to be free.

```bash
sudo certbot certonly --manual --preferred-challenges dns -d mcp.yourdomain.com
```

Replace `mcp.yourdomain.com` with your actual domain (the value of `MCP_DOMAIN` in `.env`).

**3. Create the DNS TXT record:**

Certbot will display something like:

```
Please deploy a DNS TXT record under the name:
_acme-challenge.mcp.yourdomain.com
with the following value:
AbC123xYz...
```

Go to your domain registrar's DNS settings and add:

| Type | Name | Value |
|---|---|---|
| TXT | `_acme-challenge.mcp` | `AbC123xYz...` (the value certbot shows) |

Wait 1-2 minutes for DNS propagation, then press Enter in certbot.

**4. Verify the certificates were created:**

```bash
sudo ls /etc/letsencrypt/live/mcp.yourdomain.com/
```

You should see:

```
cert.pem  chain.pem  fullchain.pem  privkey.pem  README
```

The two files used by mcp-banana are:
- `fullchain.pem` — the TLS certificate (mapped to `MCP_TLS_CERT_FILE=/certs/fullchain.pem`)
- `privkey.pem` — the private key (mapped to `MCP_TLS_KEY_FILE=/certs/privkey.pem`)

**5. Verify the certificate is valid:**

```bash
sudo openssl x509 -in /etc/letsencrypt/live/mcp.yourdomain.com/fullchain.pem -noout -dates
```

This shows the `notBefore` and `notAfter` dates.

### Certificate renewal

Let's Encrypt certificates expire after 90 days. Renew before expiry:

```bash
sudo certbot renew --manual --preferred-challenges dns
```

After renewal, restart the container to pick up the new certs:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml restart
```

### Running locally without TLS

If you see the `WARNING: TLS certificate directory not found` message on your local machine, this is expected. TLS certificates only exist on the production server. For local development, use `./scripts/run-docker-dev.sh` instead — it runs without TLS on `127.0.0.1:8847`.

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
