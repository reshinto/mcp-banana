# mcp-banana: Production-Ready MCP Server for Nano Banana

## Context

**Problem**: We need a secure MCP server that lets Claude Code generate and edit images via Google's Nano Banana API across any project, without ever exposing API keys or secrets to the AI. The server must run both locally (dev) and on a resource-constrained DigitalOcean droplet (1 vCPU, 2GB RAM, SGP1).

**Nano Banana** is Google's image generation AI (Gemini family) with 3 models:
- **Nano Banana 2** -- fast, <10s, high-volume
- **Nano Banana Pro** -- professional quality, advanced reasoning, 15-45s
- **Original Nano Banana** -- speed/efficiency optimized, 3-8s

> **Note on Gemini model IDs**: The exact Gemini model identifiers (e.g., `gemini-X.Y-flash-image-preview`) must be verified against the live Gemini API model catalog at implementation time. The aliases (`nano-banana-2`, `nano-banana-pro`, `nano-banana-original`) are stable; the underlying Gemini IDs may change. The model registry in `internal/gemini/registry.go` is the single source of truth and the only file that needs updating when IDs change.

---

## 1) Executive Recommendation

### Language Comparison

| Criterion | Go | Rust | Node.js/TypeScript |
|---|---|---|---|
| Idle memory | 5-8 MB | 2-4 MB | 30-50 MB (V8 heap) |
| Static binary size | 8-12 MB | 4-8 MB | N/A (needs ~80MB runtime) |
| Docker image (distroless) | 12-18 MB | 8-14 MB | 120-180 MB (node:slim) |
| Startup time | 5-10 ms | 3-8 ms | 200-500 ms |
| Compile time | 3-8 sec | 30-120 sec | 0 (interpreted) |
| Implementation complexity | Low | High (borrow checker) | Medium |
| MCP ecosystem fit | Strong: `mark3labs/mcp-go`, official `go-sdk` | Emerging: `mcp-rust-sdk` immature | Strongest: reference SDK is TS |
| Security surface | Small: static binary, no runtime deps | Smallest: memory-safe, no GC | Large: npm supply chain, V8 surface |
| Long-term maintainability | High | Medium (steep learning curve) | High |

### Decision: **Go**

- Project CLAUDE.md already declares Go as primary language
- Sub-10ms startup for stdio transport
- 12-18MB Docker images meeting <20MB target
- Trivial concurrency via goroutines for I/O-bound image generation
- Two production-quality MCP libraries available
- Rust offers marginal binary size savings at 10x compile time cost for a server that is fundamentally I/O-bound
- Node.js fails Docker image size target and adds unnecessary V8 overhead

### Firm Recommendations

| Decision | Choice | Rationale |
|---|---|---|
| Language | **Go** | Best balance of size, speed, ecosystem, simplicity |
| MCP library | **`mark3labs/mcp-go`** | Verified API: `NewTool`, `WithString`, `Required`, `Enum`, `ServeStdio`, `NewStreamableHTTPServer`. Explicit Streamable HTTP support. Clean builder pattern for tool schemas. |
| Gemini SDK | **`google.golang.org/genai`** | Official Google Go SDK. Supports `GenerateContent()` for Gemini multimodal image gen and `GenerateImages()` for Imagen models. |
| Transport (local) | **stdio** | Claude Code spawns process, zero network exposure |
| Transport (remote) | **Streamable HTTP on `127.0.0.1:8847`** | Bound localhost-only, accessed via SSH tunnel |
| Container base | **`gcr.io/distroless/static-debian12:nonroot`** | ~1.9MB base, no shell, no package manager |
| Auth (remote) | **Bearer token over SSH tunnel** | Defense-in-depth: SSH authenticates tunnel, bearer authenticates app |
| Secret handling | **Server-side env vars only** | Never in tool responses, logs, errors, or health checks |

> **Library choice rationale**: `mark3labs/mcp-go` is preferred over the official `github.com/modelcontextprotocol/go-sdk` for its more mature Streamable HTTP server implementation and richer builder API. **Migration policy**: If the official SDK reaches feature parity with Streamable HTTP transport and session management, evaluate migration. Pin `mark3labs/mcp-go` to a specific version in `go.sum` and vendor dependencies (`go mod vendor`) so the source is auditable.

---

## 2) Threat Model

### Assets to Protect

| Asset | Sensitivity | Impact if Compromised |
|---|---|---|
| Gemini API key | Critical | Financial abuse, quota exhaustion, attribution to owner |
| MCP auth token | High | Unauthorized image generation via the server |
| Generated images | Medium | Data exfiltration, privacy violation |
| Server availability | Medium | Denial of service |
| Prompt content | Low-Medium | Intellectual property exposure |

### Trust Boundaries

```
[Claude Code] --stdio/HTTP--> [mcp-banana server] --HTTPS--> [Gemini API]
     ^                              ^                              ^
     |                              |                              |
  Untrusted input            Trust boundary 1               Trust boundary 2
  (prompts may contain       (server validates               (server authenticates
   injection attempts)        all inputs)                     with API key)
```

### Entry Points

1. **MCP tool invocations** (primary) -- prompt text, model names, image data
2. **HTTP endpoint** (remote mode) -- Authorization header, request body
3. **Environment variables** -- loaded at startup
4. **Docker socket** (if misconfigured) -- container escape vector

### Likely Attacker Goals

| Goal | Vector | Likelihood |
|---|---|---|
| Steal API key | Prompt injection to extract env vars via error messages | High |
| Free image generation | Unauthorized access to MCP endpoint | High (if exposed publicly) |
| Denial of service | Flood requests to exhaust 1 vCPU / 2GB RAM | Medium |
| Arbitrary command execution | Command injection via prompt field | Medium |
| SSRF | Trick server into making requests to internal services | Low (server only calls Gemini API) |
| Container escape | Exploit runtime vulnerability | Low (distroless, no shell) |

### Top Risks and Mitigations

| # | Risk | Mitigation |
|---|---|---|
| 1 | **API key in tool responses** | `sanitizeResponse()` scans metadata/error fields only (NOT `image_base64`). Regex: `AIza[0-9A-Za-z_-]{35}` + env var values. See Section 3.5 for field-level targeting. |
| 2 | **API key in SDK error types** | Never call `.Error()` on raw `*googleapi.Error`. Use `errors.As()` to unwrap, then construct safe messages from scratch. See Section 5. |
| 3 | **Public port exposure** | Server binds `127.0.0.1:8847` only. Access via SSH tunnel. No public-facing port. |
| 4 | **Prompt injection** | Tool inputs are structured JSON schema fields. Server passes prompt to Gemini as data, not as instructions to mcp-banana itself. |
| 5 | **Unauthorized access (remote)** | Bearer token + SSH tunnel. Constant-time token comparison (`crypto/subtle`). |
| 6 | **Command injection** | No shell invocations anywhere. No `os/exec`. All inputs passed as structured API parameters to the Gemini SDK. |
| 7 | **DoS via semaphore starvation** | Per-model concurrency sub-limits: max 3 Pro, max 7 fast models, max 8 total. See Section 3.6. |
| 8 | **DoS on small server** | Rate limit: 30 req/min. Request timeout: 120s. Docker mem_limit: 768MB. |
| 9 | **Secret leakage in logs** | Structured slog with redaction handler. All user-supplied strings escaped for newlines. No request/response body logging. |
| 10 | **Model switching abuse** | Strict enum validation. Only 3 allowlisted model names. Gemini model IDs are internal-only. |
| 11 | **Container escape** | Distroless base (no shell), non-root user (65534), all caps dropped, `no-new-privileges`, read-only rootfs. |
| 12 | **`~/.claude.json` credential exposure** | Accepted residual risk. Mitigate: document `chmod 600 ~/.claude.json`. Any process running as the user can read this file. |
| 13 | **Supply chain (mark3labs/mcp-go)** | Pin version in `go.sum`. Vendor dependencies (`go mod vendor`). Audit before upgrades. Library has closure access to `genai.Client` via handler registration. |

---

## 3) Security Architecture

### 3.1 Secret Isolation Layers

```
Layer 1: GEMINI_API_KEY
  stdio mode:  passed via env var in ~/.claude.json, inherited by child process
  remote mode: stored in server .env / Docker secret, never transmitted to clients
  NEVER appears in: tool responses, error messages, logs, health checks, stack traces

Layer 2: MCP_AUTH_TOKEN (remote only)
  stored in: server .env + Claude Code ~/.claude.json headers
  validated: every HTTP request, constant-time comparison (crypto/subtle)
  rotated: manually via env var update + container restart
  rotation tooling: `make rotate-token` generates new token and prints update instructions

Layer 3: Transport encryption
  stdio:  OS process isolation (no network)
  remote: SSH tunnel (AES-256-GCM or ChaCha20-Poly1305)
```

### 3.2 What Claude Code Can See

| Data | Visible to Claude Code? |
|---|---|
| Tool names and schemas | Yes (by design) |
| Nano Banana model names (`nano-banana-2`, etc.) | Yes (safe aliases) |
| Generated image data (base64) | Yes (the purpose of the tool) |
| Model recommendations | Yes |
| Gemini API key | **No** |
| Gemini model IDs | **No** (internal mapping only) |
| MCP auth token value | **No** (only sent as header, never in responses) |
| Server internal errors / stack traces | **No** (wrapped into safe error messages) |
| Rate limit state | Partial (told "rate limited" with Retry-After, not internal counters) |

### 3.3 Auth Architecture (Remote)

```
Request flow:
  Claude Code --> HTTP POST /mcp
    Header: Authorization: Bearer <token>
    Body: JSON-RPC tool invocation

  mcp-banana middleware chain:
    1. MaxBytesReader(15MB) -- reject oversized requests (base64 overhead on 10MB image)
    2. extractBearerToken() -- parse Authorization header
    3. subtle.ConstantTimeCompare(token, expected) -- validate
    4. rateLimit.Allow() -- check rate limit (returns 429 + Retry-After if exceeded)
    5. modelConcurrency.Acquire(model) -- per-model concurrency limit
    6. handler(ctx) -- execute tool with context timeout
    7. sanitizeMetadata() -- scrub secrets from non-image response fields only
    8. return JSON-RPC response

  /healthz bypasses steps 4-7 entirely -- dedicated lightweight handler
```

### 3.4 What Must NOT Be Done

- **Never** log request/response bodies (may contain API keys in transit or generated content)
- **Never** include stack traces in tool error responses
- **Never** expose `/debug/pprof`, `/debug/vars`, or any introspection endpoint
- **Never** return raw Gemini API errors to Claude Code -- use `errors.As(err, &googleAPIErr)` to unwrap, construct safe messages from scratch
- **Never** call `.Error()` on raw Gemini SDK error types and forward any substring (may contain request headers with API key)
- **Never** accept arbitrary model name strings (enum validation only)
- **Never** use `fmt.Sprintf` with user input in error messages sent to clients
- **Never** bind to `0.0.0.0` in production (localhost only)
- **Never** store secrets in Docker image layers or build args
- **Never** run as root in the container
- **Never** scan `image_base64` fields with the secret-redaction regex (false positives corrupt image data)

### 3.5 Input Validation Rules

| Field | Validation |
|---|---|
| `prompt` / `instructions` | Max 10,000 chars, UTF-8 validated, no null bytes |
| `model` | Exact match against enum: `nano-banana-2`, `nano-banana-pro`, `nano-banana-original` |
| `image_base64` | Valid base64, decoded size <= 4MB, magic bytes match declared MIME type |
| `mime_type` | Exact match: `image/png`, `image/jpeg`, `image/webp` |
| `aspect_ratio` | Exact match: `1:1`, `16:9`, `9:16`, `4:3`, `3:4` |
| `priority` | Exact match: `speed`, `quality`, `balanced` |
| `task_description` | Max 5,000 chars, UTF-8 validated |

> **Image size limit rationale**: 4MB decoded (not 10MB) because actual per-request memory is ~4x the decoded size: base64 input (~5.3MB) + decoded bytes (~4MB) + Gemini SDK request serialization (~5.3MB) + Gemini response (~4-5MB) = ~19MB per request. At 8 concurrent requests, worst case is ~152MB, safely within the 768MB Docker limit.

### 3.6 Rate Limiting, Concurrency, and Timeouts

| Control | Value | Implementation |
|---|---|---|
| Request timeout | 120s | `context.WithTimeout` per Gemini API call |
| Global concurrency | 8 simultaneous | Buffered channel semaphore `make(chan struct{}, 8)` |
| Per-model concurrency | max 3 `nano-banana-pro` | Separate semaphore for slow models prevents starvation |
| Rate limit | 30 requests/min | `golang.org/x/time/rate` token bucket. Returns 429 + `Retry-After` header. |
| Payload size | 15 MB inbound (raw HTTP body) | `http.MaxBytesReader` |
| Decoded image size | 4 MB | Post-decode validation in handler |
| Response size | 20 MB outbound | Truncate if exceeded |

> **Semaphore starvation fix**: Without per-model limits, 8 concurrent `nano-banana-pro` requests (45s each) block all traffic for 45 seconds. The 3-slot Pro sub-limit ensures at least 5 slots remain for fast models.

### 3.7 Docker Hardening

```yaml
security_opt:
  - no-new-privileges:true
cap_drop:
  - ALL
read_only: true
tmpfs:
  - /tmp:noexec,nosuid,size=50m
user: "65534:65534"    # nobody:nogroup
mem_limit: 768m
cpus: "1.0"
```

### 3.8 Audit Logging

Structured JSON logs (Go `log/slog`) with:
- Request ID (UUID per request)
- Tool name invoked
- Model requested vs model used
- Response status (success/error code)
- Latency
- Rate limit decisions
- Auth failures (no token details, just "auth failed")
- **Never**: request bodies, response bodies, API keys, tokens
- **All user-supplied strings escaped** for `\n`, `\r` before insertion into log fields (prevents log injection)

---

## 4) MCP Server Design

### 4.1 Tool: `generate_image`

**Purpose**: Generate an image from a text prompt.

```go
mcp.NewTool("generate_image",
    mcp.WithDescription("Generate an image from a text prompt using Nano Banana. Returns base64-encoded image data."),
    mcp.WithString("prompt",
        mcp.Required(),
        mcp.Description("Text description of the image to generate. Max 10000 characters."),
    ),
    mcp.WithString("model",
        mcp.Description("Model to use. Omit for automatic selection (defaults to nano-banana-2). Use list_models to see options."),
        mcp.Enum("nano-banana-2", "nano-banana-pro", "nano-banana-original"),
    ),
    mcp.WithString("aspect_ratio",
        mcp.Description("Aspect ratio for the generated image."),
        mcp.Enum("1:1", "16:9", "9:16", "4:3", "3:4"),
    ),
)
```

**Output**:
```json
{
  "image_base64": "<base64 PNG data>",
  "mime_type": "image/png",
  "model_used": "nano-banana-2",
  "generation_time_ms": 4523
}
```

**Error codes** (structured, not generic):
| Code | Meaning | Claude Code Action |
|---|---|---|
| `content_policy_violation` | Prompt violates Gemini content policy | Do NOT retry with same prompt. Rephrase. |
| `quota_exceeded` | Gemini API quota exhausted | Retry after delay (Retry-After provided) |
| `model_unavailable` | Requested model is deprecated or down | Try a different model |
| `generation_failed` | Transient Gemini error | Safe to retry |
| `invalid_prompt` | Prompt failed validation | Fix prompt (too long, invalid chars) |
| `rate_limited` | Server rate limit exceeded | Retry after Retry-After seconds |
| `server_busy` | All concurrency slots occupied | Retry after brief delay |
| `server_error` | Unexpected internal error | Report to server admin |

### 4.2 Tool: `edit_image`

**Purpose**: Edit an existing image using text instructions.

```go
mcp.NewTool("edit_image",
    mcp.WithDescription("Edit an existing image using text instructions. Provide base64 image data and describe changes."),
    mcp.WithString("image_base64",
        mcp.Required(),
        mcp.Description("Base64-encoded image data. Max 4MB decoded size."),
    ),
    mcp.WithString("mime_type",
        mcp.Required(),
        mcp.Description("MIME type of the input image."),
        mcp.Enum("image/png", "image/jpeg", "image/webp"),
    ),
    mcp.WithString("instructions",
        mcp.Required(),
        mcp.Description("Text instructions describing edits to apply. Max 10000 characters."),
    ),
    mcp.WithString("model",
        mcp.Description("Model to use. Defaults to nano-banana-2 for edits."),
        mcp.Enum("nano-banana-2", "nano-banana-pro", "nano-banana-original"),
    ),
)
```

**Output**: Same schema as `generate_image`. Same error codes.

### 4.3 Tool: `list_models`

**Purpose**: List all available Nano Banana models with capabilities.

```go
mcp.NewTool("list_models",
    mcp.WithDescription("List all available Nano Banana models with capabilities and recommended use cases."),
)
```

**Output**:
```json
{
  "models": [
    {
      "id": "nano-banana-2",
      "description": "Fast, high-volume image generation. Under 10 seconds.",
      "capabilities": ["generate", "edit"],
      "typical_latency": "5-10s",
      "best_for": "Iterative work, drafts, batch generation",
      "status": "available"
    },
    {
      "id": "nano-banana-pro",
      "description": "Professional quality with advanced reasoning. 15-45 seconds.",
      "capabilities": ["generate", "edit"],
      "typical_latency": "15-45s",
      "best_for": "Final assets, photorealistic images, complex scenes",
      "status": "available"
    },
    {
      "id": "nano-banana-original",
      "description": "Speed and efficiency optimized. 3-8 seconds.",
      "capabilities": ["generate", "edit"],
      "typical_latency": "3-8s",
      "best_for": "Quick previews, high-volume batch work",
      "status": "available"
    }
  ]
}
```

### 4.4 Tool: `recommend_model`

**Purpose**: Server recommends optimal model based on task description. Advisory only -- does not execute generation.

```go
mcp.NewTool("recommend_model",
    mcp.WithDescription("Get a model recommendation based on task description. Returns the best model for your task. Advisory only -- you must still call generate_image with the recommended model."),
    mcp.WithString("task_description",
        mcp.Required(),
        mcp.Description("Description of what you want to accomplish. Max 5000 characters."),
    ),
    mcp.WithString("priority",
        mcp.Description("What matters most for this task."),
        mcp.Enum("speed", "quality", "balanced"),
    ),
)
```

**Output**:
```json
{
  "recommended_model": "nano-banana-pro",
  "reason": "Task requires photorealistic quality with complex multi-subject composition.",
  "alternatives": [
    {
      "model": "nano-banana-2",
      "tradeoff": "Faster (5-10s vs 15-45s) but lower fidelity for complex scenes"
    }
  ]
}
```

> **Tool count rationale**: 4 tools (not 5). `get_model_info` is merged into `list_models` since all model info is static and fits in a single response. Fewer tools = less cognitive load for Claude Code.

---

## 5) Secret Isolation Design

### What Claude Code Can See vs Cannot See

| Item | Claude Code Sees | Stored Where |
|---|---|---|
| Tool schemas | Yes | Returned via MCP `tools/list` |
| Model names (aliases) | Yes | Hardcoded in server |
| Generated images | Yes | Returned as base64 in tool results |
| GEMINI_API_KEY | **Never** | Server env var / Docker secret |
| MCP_AUTH_TOKEN | **Never in responses** | Server env + Claude Code config header |
| Gemini model IDs | **Never** | Internal mapping in `internal/gemini/registry.go` |
| Error details from Gemini | **Never** | Wrapped into safe error codes |
| Server config | **Never** | Environment variables only |

### Mediation Pattern

```
Claude Code: "generate_image(prompt='a cat', model='nano-banana-pro')"
     |
     v
mcp-banana server:
  1. Validate: model in allowlist? YES
  2. Map: "nano-banana-pro" -> internal Gemini model ID (never exposed)
  3. Call: genai.Client.GenerateContent(ctx, geminiModelID, prompt)
     (API key added by genai SDK from env, never visible to caller)
  4. Receive: Gemini response with image
  5. Handle errors safely:
     - errors.As(err, &googleAPIErr) to unwrap
     - Map to safe error codes (content_policy_violation, quota_exceeded, etc.)
     - NEVER forward raw error text from SDK
  6. Sanitize: scan metadata fields (NOT image_base64) for secret patterns
  7. Return: { image_base64: "...", model_used: "nano-banana-pro" }
```

### Leakage Prevention

| Vector | Prevention |
|---|---|
| Tool responses | `sanitizeMetadata()` scans only non-image string fields for API key regex + env var values |
| Error messages | All Gemini errors unwrapped via `errors.As()`, mapped to safe codes. Raw `.Error()` text never forwarded. |
| Logs | `log/slog` with custom handler: redacts secret patterns, escapes `\n`/`\r` in user strings |
| Stack traces | `recover()` in HTTP handler; panics logged server-side, client gets `"server_error"` |
| Health check | `GET /healthz` returns only `{"status":"ok"}` -- no config, no versions, no env info. Bypasses all middleware. |
| Debug endpoints | None compiled in production builds |
| Base64 image data | **Excluded from sanitization** -- generated by server, cannot contain real secrets, scanning causes false-positive corruption |

### Startup Validation

At startup, before accepting connections:
1. Validate `GEMINI_API_KEY` is present and non-empty
2. Validate `GEMINI_API_KEY` format matches expected pattern (fail fast with safe log: "GEMINI_API_KEY format invalid")
3. If HTTP transport: validate `MCP_AUTH_TOKEN` is present, non-empty, and >= 32 chars
4. Optionally: call Gemini's model list API to verify configured model IDs still exist. Log `WARN` for any missing models (using alias names, not Gemini IDs).

---

## 6) Transport and Networking Recommendation

### Local Development: stdio (recommended default)

```
Claude Code --stdin/stdout--> mcp-banana process --HTTPS--> Gemini API
```

- Claude Code spawns mcp-banana as a child process
- Communication is in-process JSON-RPC over stdin/stdout
- Zero network surface -- no ports, no listeners, no firewall rules
- API key passed as env var via `~/.claude.json` config
- **This is the recommended mode for most users**

### Remote (DigitalOcean): Streamable HTTP via SSH tunnel

```
Laptop:
  Claude Code --> http://localhost:8847/mcp
       |
       | SSH tunnel (port 22, already authenticated)
       v
Droplet (SGP1):
  sshd:22 --> forwards to 127.0.0.1:8847
                    |
             mcp-banana (binds 127.0.0.1:8847 only)
                    |
                    v
             Gemini API (HTTPS)
```

**Why SSH tunnel is the single best choice:**

| Approach | Verdict | Reason |
|---|---|---|
| **SSH tunnel** | **Selected** | Zero additional infrastructure. Reuses existing sshd on port 22. Encryption + auth for free. No ports exposed publicly. Works out of the box on any DigitalOcean droplet. |
| mTLS | Rejected | Requires PKI setup, cert distribution, renewal automation. Overkill for single-user. |
| Reverse proxy (nginx/Caddy) | Rejected | Must expose a public port. TLS cert management. Additional attack surface and memory overhead on 2GB server. |
| WireGuard VPN | Acceptable fallback | Good for multi-user scenarios. Adds kernel module dependency and persistent config for single-user case. |
| "Hidden" port | **Rejected absolutely** | Security through obscurity is not security. |

**Port choice: 8847**
- Avoids all listed in-use ports (22, 80, 443, 3000, 3004, 8080, 9000, 9001)
- Avoids 8888 (Jupyter Notebook default, likely to conflict on a dev droplet)
- Bound to `127.0.0.1` only -- not reachable from the internet

**Is exposing the MCP server publicly advisable?** **No.** Even with bearer tokens and rate limiting, a public HTTP endpoint on a 1 vCPU / 2GB RAM server is trivially DoS-able.

### SSH Tunnel Commands

```bash
# Simple (run once per session)
ssh -N -L 8847:127.0.0.1:8847 user@droplet-ip

# Persistent with auto-reconnect
autossh -M 0 -N -L 8847:127.0.0.1:8847 user@droplet-ip

# Background with SSH config (~/.ssh/config)
Host banana-tunnel
    HostName <droplet-ip>
    User <user>
    LocalForward 8847 127.0.0.1:8847
    ServerAliveInterval 60
    ServerAliveCountMax 3
```

### Claude Code Configuration (cross-project, in `~/.claude.json`)

**stdio mode (local):**
```json
{
  "mcpServers": {
    "banana": {
      "command": "/usr/local/bin/mcp-banana",
      "args": ["--transport", "stdio"],
      "env": {
        "GEMINI_API_KEY": "AIza..."
      },
      "type": "stdio"
    }
  }
}
```

**Streamable HTTP mode (remote via tunnel):**
```json
{
  "mcpServers": {
    "banana": {
      "type": "streamable-http",
      "url": "http://localhost:8847/mcp",
      "headers": {
        "Authorization": "Bearer <pre-shared-token>"
      }
    }
  }
}
```

> **Verified**: Claude Code supports `"type": "streamable-http"` (confirmed). The older `"sse"` type is deprecated (April 2026 sunset). Cross-project config in `~/.claude.json` is supported with `--scope user`.

### SSH Tunnel Reconnection and MCP Sessions

When an SSH tunnel drops and `autossh` reconnects, the TCP connection to `127.0.0.1:8847` is new. If `mark3labs/mcp-go`'s Streamable HTTP transport maintains per-session state via `Mcp-Session-Id` headers, Claude Code must re-initialize the MCP connection after tunnel reconnection. The server should be configured stateless where possible to minimize this friction.

---

## 7) Docker and Deployment Design

### Dockerfile

```dockerfile
# Stage 1: Builder
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache ca-certificates git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /build/mcp-banana \
    ./cmd/mcp-banana/

# Stage 2: Production runner
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /build/mcp-banana /usr/local/bin/mcp-banana
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
USER 65534:65534
EXPOSE 8847
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/mcp-banana", "--healthcheck"]
ENTRYPOINT ["/usr/local/bin/mcp-banana"]
CMD ["--transport", "http", "--addr", "127.0.0.1:8847"]
```

**Expected image size**: ~14 MB (distroless static ~1.9MB + Go binary ~12MB with stripped symbols)

> **Health check note**: The `--healthcheck` flag makes the binary perform an HTTP GET to its own `/healthz` endpoint. `/healthz` is a dedicated lightweight handler that bypasses rate limiting and concurrency semaphores entirely, preventing cascading restarts when the server is busy with long-running Gemini requests.

### docker-compose.yml

```yaml
services:
  mcp-banana:
    build: .
    image: mcp-banana:latest
    container_name: mcp-banana
    restart: unless-stopped
    stop_grace_period: 120s
    ports:
      - "127.0.0.1:8847:8847"
    environment:
      - GEMINI_API_KEY=${GEMINI_API_KEY}
      - MCP_AUTH_TOKEN=${MCP_AUTH_TOKEN}
      - MCP_LOG_LEVEL=info
      - GOMEMLIMIT=700MiB
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
    read_only: true
    tmpfs:
      - /tmp:noexec,nosuid,size=50m
    mem_limit: 768m
    cpus: "1.0"
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

> **`stop_grace_period: 120s`**: Docker default is 10s SIGTERM-to-SIGKILL. Nano Banana Pro requests take up to 45s. The 120s grace period allows in-flight requests to complete. The server implements `http.Server.Shutdown(ctx)` with a matching 120s timeout context in the SIGTERM handler.

### .dockerignore

```
.git
.claude
.env
*.md
LICENSE
vendor/
```

### .env.example

```
GEMINI_API_KEY=
MCP_AUTH_TOKEN=
MCP_LOG_LEVEL=info
```

### Local Deployment

```bash
# Build binary directly
go build -o /usr/local/bin/mcp-banana ./cmd/mcp-banana/

# Or via Docker for testing
docker compose up -d --build
```

### DigitalOcean Deployment

```bash
# On droplet -- SSH hardening first
sudo sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config
sudo sed -i 's/#PermitRootLogin yes/PermitRootLogin no/' /etc/ssh/sshd_config
sudo systemctl restart sshd

# Deploy
sudo mkdir -p /opt/mcp-banana && cd /opt/mcp-banana
git clone <repo-url> .
cp .env.example .env
# Edit .env: GEMINI_API_KEY=AIza..., MCP_AUTH_TOKEN=$(openssl rand -hex 32)
chmod 600 .env
docker compose up -d --build

# Verify
curl -s http://127.0.0.1:8847/healthz  # {"status":"ok"}
docker stats mcp-banana                 # check resource usage
```

---

## 8) Performance and Footprint Optimization

### Memory Targets (corrected)

| Metric | Target | How |
|---|---|---|
| Idle memory | <10 MB | Go runtime ~5MB baseline; lazy Gemini client init; no pre-allocated buffers |
| Per-request (generate) | ~20 MB | base64 response (~5MB) + decoded image (~4MB) + SDK overhead (~8MB) + goroutine stack |
| Per-request (edit) | ~38 MB | Input image (base64 + decoded ~9MB) + Gemini request (~9MB) + response (~16MB) + goroutine |
| Worst case (8 concurrent, mix) | ~250 MB | 3 edit + 5 generate = ~114MB + ~100MB + overhead |
| Docker memory ceiling | 768 MB | Headroom for GC, OS, and burst |

> **Why 768MB not 512MB**: Original plan underestimated per-request memory by ~4x. Base64 encoding, SDK serialization, and response buffers mean each request allocates 3-4 copies of the image data. The corrected model accounts for this.

### CPU Targets

| Metric | Approach |
|---|---|
| Concurrency limit | 8 global, 3 Pro sub-limit (I/O-bound, not CPU-bound) |
| GC pressure | `GOMEMLIMIT=700MiB` tells GC to be aggressive before hitting Docker limit |
| No hot loops | All work is "send request to Gemini, wait for response" |

### Binary and Image Size

| Optimization | Savings |
|---|---|
| `-ldflags="-s -w"` | Strips debug info and DWARF symbols (~30% smaller) |
| `-trimpath` | Removes local file paths from binary |
| `CGO_ENABLED=0` | Static binary, no libc dependency |
| Distroless base | ~1.9MB vs ~80MB for ubuntu-slim |
| `.dockerignore` | Keeps .git, .claude, docs out of build context |

### Connection Efficiency

| Control | Value |
|---|---|
| HTTP server timeouts | Read: 30s, Write: 150s (accounts for image gen), Idle: 60s |
| Gemini client | Reuse single `*genai.Client` instance (connection pooling built in) |
| Keep-alive | Enabled for Gemini HTTPS connections |
| Request timeout | 120s per Gemini call via `context.WithTimeout` |

---

## 9) Model Selection Policy Design

### Policy Matrix

| Priority | Task Keywords | Recommended Model |
|---|---|---|
| `speed` | (any) | `nano-banana-original` |
| `quality` | (any) | `nano-banana-pro` |
| `balanced` | "professional", "photorealistic", "detailed", "complex", "final" | `nano-banana-pro` |
| `balanced` | "quick", "draft", "sketch", "iterate", "batch", "preview" | `nano-banana-original` |
| `balanced` | (default / no keyword match) | `nano-banana-2` |
| (omitted) | (same as `balanced`) | follows balanced rules |

### Enforcement Rules

1. **Claude Code may suggest** a model name via the `model` field in `generate_image` / `edit_image`
2. **Server validates** the name against the 3-entry allowlist enum. Unknown values are rejected with `"invalid_model"` error
3. **If model is omitted**, server auto-selects `nano-banana-2` as the safe default
4. **Gemini model IDs are never exposed** -- the mapping from alias to ID is internal to `internal/gemini/registry.go`
5. **`recommend_model` is advisory** -- it returns a recommendation but does not execute anything
6. **All selections are logged** (model requested, model used, reason) for audit
7. **Model deprecation detection**: On startup, optionally verify model IDs against Gemini's model list API. At runtime, if a model returns a "not found" error, return `model_unavailable` error code with the alias name (not the Gemini ID)

### How Claude Code Uses This

Typical flow:
1. Claude Code calls `recommend_model(task_description="create a professional product photo", priority="quality")`
2. Server returns `{ recommended_model: "nano-banana-pro", reason: "..." }`
3. Claude Code calls `generate_image(prompt="...", model="nano-banana-pro")`
4. Server validates, maps to Gemini ID, generates, returns image

Or shortcut:
1. Claude Code calls `generate_image(prompt="quick sketch of a logo")` (no model specified)
2. Server auto-selects `nano-banana-2` as default
3. Returns image with `model_used: "nano-banana-2"` so Claude Code knows what was used

---

## 10) Project Structure

```
mcp-banana/
├── cmd/
│   └── mcp-banana/
│       └── main.go                 # Entry point: flag parsing, transport selection, graceful shutdown
├── internal/
│   ├── server/
│   │   ├── server.go               # MCPServer setup, tool registration, health check handler
│   │   ├── server_test.go
│   │   ├── middleware.go            # Rate limiting, concurrency, timeout, auth, logging
│   │   └── middleware_test.go
│   ├── tools/
│   │   ├── generate.go             # generate_image handler
│   │   ├── generate_test.go
│   │   ├── edit.go                 # edit_image handler
│   │   ├── edit_test.go
│   │   ├── models.go               # list_models handler
│   │   ├── models_test.go
│   │   ├── recommend.go            # recommend_model handler
│   │   └── recommend_test.go
│   ├── gemini/
│   │   ├── client.go               # Gemini API client wrapper with safe error handling
│   │   ├── client_test.go
│   │   ├── registry.go             # Model registry: alias-to-ID mapping (single source of truth)
│   │   └── errors.go               # Gemini error unwrapping and safe error code mapping
│   ├── policy/
│   │   ├── selector.go             # Model recommendation algorithm
│   │   └── selector_test.go
│   ├── security/
│   │   ├── sanitize.go             # Output sanitization (metadata fields only, NOT image data)
│   │   ├── sanitize_test.go
│   │   ├── validate.go             # Input validation functions
│   │   └── validate_test.go
│   └── config/
│       ├── config.go               # Load + validate env vars at startup (fail-fast)
│       └── config_test.go
├── Dockerfile
├── docker-compose.yml
├── .dockerignore
├── .env.example
├── .gitignore
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── LICENSE
```

### Key Design Decisions

- **`internal/`** prevents external import of implementation details
- **Tests co-located** with source (per project `testing.md` rules)
- **Feature-grouped** directories (per `architecture.md`: "group by feature or domain")
- **Flattened middleware**: `server/middleware.go` instead of separate `middleware/` package (3 concerns in one file is simpler for this scope)
- **`internal/gemini/errors.go`**: Dedicated file for safe Gemini error unwrapping -- critical security boundary
- **`internal/gemini/registry.go`** is the single source of truth for model alias-to-ID mapping
- **4 tools** (not 5): `get_model_info` merged into `list_models`
- **Vendored dependencies**: `go mod vendor` for supply chain auditability

---

## 11) Production Readiness Checklist

### Security
- [ ] All tool inputs validated against strict JSON schemas with enum constraints
- [ ] Output sanitization active on metadata fields only (NOT image_base64)
- [ ] Gemini SDK errors unwrapped via `errors.As()` -- raw `.Error()` text never forwarded
- [ ] Bearer token auth with constant-time comparison (`crypto/subtle`)
- [ ] Server binds `127.0.0.1` only -- no public port exposure
- [ ] Docker: non-root user, all caps dropped, no-new-privileges, read-only rootfs
- [ ] No debug endpoints, no pprof, no expvar in production
- [ ] Log redaction active; user-supplied strings escaped for `\n`/`\r`
- [ ] No shell invocations or `os/exec` usage anywhere
- [ ] Dependencies vendored (`go mod vendor`) for supply chain auditability
- [ ] SSH hardening on droplet: `PasswordAuthentication no`, `PermitRootLogin no`
- [ ] `~/.claude.json` set to `chmod 600`

### Deployment
- [ ] Multi-stage Dockerfile producing <20MB image
- [ ] `docker-compose.yml` with all hardening flags + `stop_grace_period: 120s`
- [ ] `.env.example` documents all required environment variables
- [ ] Health check `/healthz` bypasses rate limiter and semaphore
- [ ] SSH tunnel documented and tested for remote access
- [ ] Container restart policy: `unless-stopped`
- [ ] Log rotation configured (10MB x 3 files)

### Performance
- [ ] Idle memory < 10 MB confirmed
- [ ] Global concurrency limited to 8, Pro sub-limit at 3
- [ ] Rate limited to 30 requests/minute with 429 + Retry-After
- [ ] Request timeout 120s enforced
- [ ] Decoded image size limited to 4 MB
- [ ] `GOMEMLIMIT=700MiB` set for GC optimization
- [ ] Docker `mem_limit: 768m` with corrected memory model

### Observability
- [ ] Structured JSON logging via `log/slog`
- [ ] Request ID on every request
- [ ] Tool invocation + model selection logged
- [ ] Auth failures logged (no secret details)
- [ ] `docker stats` / `docker compose logs` for operational monitoring

### Secret Handling
- [ ] `GEMINI_API_KEY` never appears in any tool response, log, or error
- [ ] `MCP_AUTH_TOKEN` never appears in any tool response
- [ ] Gemini model IDs never exposed to Claude Code
- [ ] `.env` in `.gitignore` and `.dockerignore`
- [ ] No secrets in Docker image layers or build args
- [ ] Startup validation: fail fast if API key format is invalid

### Testing
- [ ] Unit tests for all tool handlers
- [ ] Unit tests for input validation (including boundary values)
- [ ] Unit tests for output sanitization (adversarial inputs + false-positive prevention on base64)
- [ ] Unit tests for Gemini error unwrapping (verify no raw error text leaks)
- [ ] Unit tests for model recommendation algorithm
- [ ] Integration test: tool invocation end-to-end (with mock Gemini client)
- [ ] Coverage meets 80/75/80/80 thresholds
- [ ] CI: `golangci-lint run` -> `gofmt -w .` -> `go vet ./...` -> `go test ./...`

### Failure Handling
- [ ] Gemini API errors mapped to specific safe error codes (not generic "generation_failed")
- [ ] Panic recovery in HTTP handler
- [ ] Graceful shutdown: `http.Server.Shutdown(ctx)` with 120s timeout on SIGTERM
- [ ] `stop_grace_period: 120s` in docker-compose.yml
- [ ] Rate limit returns HTTP 429 with `Retry-After` header
- [ ] Concurrency limit returns `server_busy` error
- [ ] Model deprecation returns `model_unavailable` with alias name

### Rotation / Maintenance
- [ ] API key rotation: update `.env`, restart container
- [ ] Auth token rotation: `make rotate-token` + update `~/.claude.json`
- [ ] Image updates: `docker compose up -d --build && docker image prune -f`
- [ ] Startup model verification catches deprecated Gemini model IDs

---

## 12) Final Recommendation

| Decision | Choice |
|---|---|
| **Architecture** | Single Go binary MCP server with stdio + Streamable HTTP dual transport, Gemini API backend, server-side secret isolation |
| **Language** | **Go** -- 12MB binary, 14MB Docker image, 5ms startup, trivial concurrency, mature MCP library |
| **Transport** | **stdio for local** (zero attack surface), **Streamable HTTP via SSH tunnel for remote** (zero public ports) |
| **Authentication** | **Bearer token over SSH tunnel** -- SSH handles transport auth, bearer token is defense-in-depth |
| **Deployment** | **Docker Compose on DigitalOcean** with distroless image, all hardening flags, localhost-only binding, SSH tunnel for remote access |

### Top 5 Mistakes to Avoid

1. **Scanning image_base64 with secret-redaction regex** -- False positives corrupt image data silently. Sanitize metadata fields only.

2. **Forwarding raw Gemini SDK error text** -- `*googleapi.Error` types can serialize request headers containing the API key. Always unwrap with `errors.As()` and construct safe messages from scratch.

3. **No per-model concurrency limits** -- Without sub-limits, slow models (45s Pro requests) starve fast models. Always reserve slots for fast models.

4. **Docker default stop timeout (10s)** -- Kills in-flight 45s image generation requests. Set `stop_grace_period: 120s` and implement matching graceful shutdown.

5. **Storing secrets in `.mcp.json` (project-scoped)** -- Project-scoped config is committed to git. API keys and tokens must go in `~/.claude.json` (user-scoped, `chmod 600`) or server environment variables only.

---

## Implementation Plan

### Phase 1: Project scaffolding
- Initialize Go module
- Create directory structure per section 10
- Create `Makefile` with build, test, lint, rotate-token targets
- Create `Dockerfile`, `docker-compose.yml`, `.dockerignore`, `.env.example`, `.gitignore`
- Create `README.md`

### Phase 2: Core server
- Implement `internal/config/config.go` -- env var loading and startup validation (fail-fast)
- Implement `internal/gemini/registry.go` -- model registry and alias mapping
- Implement `internal/gemini/errors.go` -- safe Gemini error unwrapping
- Implement `internal/gemini/client.go` -- Gemini API client wrapper
- Implement `internal/server/server.go` -- MCP server setup with `mark3labs/mcp-go`, health check handler
- Implement `cmd/mcp-banana/main.go` -- entry point with transport flag, graceful shutdown (120s)

### Phase 3: Security layer
- Implement `internal/security/validate.go` -- input validation
- Implement `internal/security/sanitize.go` -- metadata-only output sanitization
- Implement `internal/server/middleware.go` -- auth, rate limiting (with 429 + Retry-After), per-model concurrency, timeout, logging with redaction

### Phase 4: Tool handlers
- Implement `internal/tools/generate.go` -- generate_image
- Implement `internal/tools/edit.go` -- edit_image
- Implement `internal/tools/models.go` -- list_models
- Implement `internal/policy/selector.go` -- recommendation algorithm
- Implement `internal/tools/recommend.go` -- recommend_model

### Phase 5: Testing
- Unit tests for all packages (80%+ coverage)
- Integration tests with mock Gemini client
- Security-focused tests: sanitization false-positive prevention on base64, error unwrapping leak prevention, adversarial input validation
- CI pipeline validation

### Phase 6: Documentation and deployment
- README with setup, configuration, deployment instructions
- Update `.claude/rules/architecture.md` with actual architecture
- Test local stdio deployment
- Test Docker deployment
- Test SSH tunnel remote deployment
- Vendor dependencies: `go mod vendor`

### Verification

```bash
# Build
go build -o mcp-banana ./cmd/mcp-banana/

# Quality gate
golangci-lint run
gofmt -w .
go vet ./...
go test ./...

# Local test (stdio)
echo '{"jsonrpc":"2.0","method":"tools/list","id":1}' | ./mcp-banana --transport stdio

# Docker test
docker compose up -d --build
curl -H "Authorization: Bearer $MCP_AUTH_TOKEN" http://localhost:8847/healthz

# Integration test (requires GEMINI_API_KEY)
GEMINI_API_KEY=test ./mcp-banana --transport stdio <<< '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_models"},"id":1}'
```
