# Root Files Reference

This document describes every file and directory at the project root.

---

## Source Code

### `cmd/`

Entry point for the `mcp-banana` binary. Contains a single package at `cmd/mcp-banana/main.go`.

`main.go` parses command-line flags (`--transport`, `--addr`, `--healthcheck`, `--version`), loads configuration, validates the model registry, registers secrets with the sanitizer, initializes the Gemini client and client cache, constructs the MCP server, and starts either the stdio or HTTP transport.

No business logic lives here. `cmd/` only wires dependencies.

### `internal/`

All application code. Go's `internal/` convention prevents these packages from being imported by any code outside this module.

| Package | Responsibility |
|---|---|
| `config/` | Environment variable loading and `Config` struct validation |
| `gemini/` | Gemini API client, model registry, error mapping, client cache, context helpers |
| `oauth/` | OAuth 2.1 authorization: store, PKCE, handlers, dynamic client registration, provider delegation |
| `policy/` | Model recommendation logic (keyword matching and priority selection) |
| `security/` | Input validation and output sanitization — the security boundary |
| `server/` | HTTP routing, middleware chain (auth, rate limiting, concurrency, body size) |
| `tools/` | MCP tool handler factories (`generate_image`, `edit_image`, `list_models`, `recommend_model`) |

### `go.mod` and `go.sum`

Go module definition and dependency checksums. `go.sum` records cryptographic hashes of every dependency to prevent supply chain tampering.

---

## Build and Operations

### `Makefile`

All common development commands as phony targets:

| Target | Description |
|---|---|
| `build` | Compile the binary to `./mcp-banana` with CGO disabled and debug info stripped |
| `test` | Run all tests with race detection and coverage (`go test -race -coverprofile=coverage.out ./... -v`) |
| `lint` | Run `golangci-lint run` |
| `fmt` | Format all Go source files with `gofmt -w .` |
| `fmt-check` | Fail if any file needs formatting — used in CI |
| `vet` | Run `go vet ./...` static analysis |
| `run-stdio` | Build and run in stdio mode |
| `run-http` | Build and run in HTTP mode on `0.0.0.0:8847` |
| `clean` | Remove the compiled binary and `coverage.out` |
| `rotate-token` | Generate a new auth token with `openssl rand -hex 32` and print rotation instructions |
| `quality-gate` | Run lint → fmt-check → vet → test in order — required before every commit |

Build flags: `-ldflags="-s -w"` strips the symbol table and debug info (smaller binary). `-trimpath` removes absolute file paths (reproducible builds).

### `Dockerfile`

Two-stage Docker build targeting Linux/amd64.

**Stage 1 (builder):** Uses `golang:1.25-alpine`. Copies `go.mod` and `go.sum` before source (maximizes layer cache reuse). Builds with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` and the same optimization flags as `make build`.

**Stage 2 (runner):** Uses `gcr.io/distroless/static-debian12:nonroot`. No shell, no package manager, no writable filesystem except `/tmp`. The compiled binary is the only thing copied from the builder stage. The container runs as `nonroot` (UID 65532).

Port 8847 is exposed. Default command: `--transport http --addr 0.0.0.0:8847`.

Health check runs every 30 seconds:
```
/usr/local/bin/mcp-banana --healthcheck --addr 127.0.0.1:8847
```
Timeout: 5s. Startup grace period: 10s. Retries: 3 before marking unhealthy.

### `docker-compose.yml`

Development compose file. Defines a single service named `mcp-banana`:

- Builds from the local `Dockerfile`
- Binds port 8847 to `127.0.0.1:8847` (loopback only — not publicly exposed)
- Loads environment from `.env`
- Restarts automatically unless explicitly stopped (`restart: unless-stopped`)
- Grace period of 120 seconds before forcibly killing on shutdown (`stop_grace_period: 120s`)
- Memory limit: 768 MB
- Logs captured by Docker's `json-file` driver, 10 MB per file, 3 files max

Start dev server: `./scripts/run-docker-dev.sh`

### `docker-compose.prod.yml`

Production overlay. Extends `docker-compose.yml` with:

- Port bound to `0.0.0.0:8847` (all interfaces — publicly accessible)
- TLS certificate volume mounted read-only from `/etc/letsencrypt/live/mcp.terencekong.net`

Used with: `docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build`

Start production server: `./scripts/run-docker-prod.sh`

### `scripts/`

Three helper scripts for running the server:

| Script | Description |
|---|---|
| `run-local.sh` | Loads `.env`, builds the binary, starts in stdio mode |
| `run-docker-dev.sh` | Builds and starts the Docker container in dev mode (`127.0.0.1:8847`) |
| `run-docker-prod.sh` | Builds and starts the Docker container in production mode (`0.0.0.0:8847`, TLS, overlay) |

---

## Configuration

### `.env.example`

Template for runtime configuration. Copy to `.env` and fill in values:

```bash
cp .env.example .env
```

Documents every environment variable with description, default, and constraints. See [setup-and-operations.md](setup-and-operations.md) for the full configuration reference.

### `.mcp.json`

Project-scoped Claude Code MCP server definition. Currently empty (`{"mcpServers": {}}`). When populated, it allows Claude Code to discover the server without per-user configuration. Each developer provides their own `GEMINI_API_KEY` via their shell environment or user-scoped Claude Code config.

---

## Code Quality

### `.golangci.yml`

golangci-lint v2 configuration. Enabled linters:

- `errcheck` — flags unchecked error return values
- `govet` (all checks except `fieldalignment`) — static analysis
- `staticcheck` — advanced static analysis
- `unused` — flags unused exports
- `ineffassign` — flags assignments whose values are never used

`stylecheck` and `revive` are not enabled. Do not re-enable them.

---

## CI/CD

### `.github/workflows/ci.yml`

CI workflow triggered on feature branch pushes and PRs to `main`:

1. Lint — `golangci-lint run`
2. Format check — `gofmt -w .`
3. Type check — `go vet ./...`
4. Unit tests — `go test -race -coverprofile=coverage.out ./...` with 80% coverage enforcement
5. Binary build — `go build ./cmd/mcp-banana/`

Action versions are pinned to commit SHAs.

---

## Documentation

### `docs/`

All project documentation. Files committed to the repository:

| File | Contents |
|---|---|
| `architecture.md` | System design, package layout, data flow diagram |
| `authentication.md` | Bearer token and OAuth 2.1 setup |
| `claude-code-integration.md` | Claude Code configuration and usage |
| `go-guide.md` | Go language concepts with codebase examples |
| `models.md` | Model aliases, latency, use cases, verification procedure |
| `root-files.md` | This file |
| `security.md` | Threat model and security controls |
| `setup-and-operations.md` | Prerequisites, local setup, Docker deployment, environment variables |
| `testing.md` | Test runner, coverage, patterns, security test requirements |
| `tools-reference.md` | Full schema for all four MCP tools |
| `troubleshooting.md` | Common failure modes and diagnostics |

The `assets/` and `diagrams/` subdirectories contain images used by documentation files.

---

## Version Control

### `.gitignore`

Excludes: compiled binary (`mcp-banana`), `.env` files, IDE directories (`.vscode/`, `.idea/`), OS files (`.DS_Store`, `Thumbs.db`), test coverage files (`coverage.out`, `cov*.out`), Claude local settings (`.claude/settings.local.json`), `.superpowers/`, and `.claude/worktrees`.

### `.gitattributes`

Enforces LF line endings for `*.go`, `Makefile`, `Dockerfile`, `*.sh`, `*.yml`, `*.yaml` to prevent CI failures caused by CRLF line endings on Windows. All other text files use `text=auto eol=lf`.

### `.dockerignore`

Excludes from the Docker build context: `.git/`, `.claude/`, `.env`, `*.md`, `LICENSE`, `docs/`. This keeps the build context small and prevents secrets or local config from being copied into the image.

---

## Licensing

### `LICENSE`

MIT License. See the file for full terms.

### `CONTRIBUTING.md`

Development workflow, coding standards, PR process, branch naming, quality gate, and OAuth development setup. See [CONTRIBUTING.md](../CONTRIBUTING.md).
