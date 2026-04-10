# Root Files Reference

This document describes every file and directory at the project root.

## Source Code

### `cmd/`

Entry point for the `mcp-banana` binary. Contains a single subdirectory `cmd/mcp-banana/main.go`.

`main.go` parses command-line flags, loads configuration, validates the model registry, registers secrets with the sanitizer, initializes the Gemini client, creates the MCP server, and starts either the stdio or HTTP transport. See [architecture.md](architecture.md) for the full startup sequence.

### `internal/`

All application code. Go's `internal/` convention prevents these packages from being imported by any code outside this module. Subdirectories:

- `config/` - environment variable loading and validation
- `gemini/` - Gemini API client, model registry, error mapping
- `policy/` - model recommendation logic
- `security/` - input validation and output sanitization
- `server/` - HTTP routing and middleware
- `tools/` - MCP tool handler factories

### `go.mod` and `go.sum`

Go module definition and dependency checksums. `go.sum` records cryptographic hashes of every dependency to prevent supply chain tampering.

## Build and Operations

### `Makefile`

Defines all common development commands as phony targets:

| Target | Command | Description |
|---|---|---|
| `build` | `CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o mcp-banana ./cmd/mcp-banana/` | Compile the binary with CGO disabled and debug info stripped |
| `test` | `go test -coverprofile=coverage.out -race ./... -v` | Run tests with race detection and coverage |
| `lint` | `golangci-lint run` | Run configured linters |
| `fmt` | `gofmt -w .` | Format all Go source files |
| `fmt-check` | `gofmt -l .` then exit 1 if output is non-empty | Fail if any file needs formatting (used in CI) |
| `vet` | `go vet ./...` | Run static analysis |
| `run-stdio` | `build` then `./mcp-banana --transport stdio` | Build and run in stdio mode |
| `run-http` | `build` then `./mcp-banana --transport http --addr 0.0.0.0:8847` | Build and run in HTTP mode |
| `clean` | `rm -f mcp-banana coverage.out` | Remove build artifacts |
| `rotate-token` | `openssl rand -hex 32` then print instructions | Generate a new auth token and print rotation steps |
| `quality-gate` | `lint` + `fmt-check` + `vet` + `test` | Run the full CI sequence |

The build flags `-ldflags="-s -w"` strip the symbol table and debug info, reducing binary size. `-trimpath` removes absolute file paths from the binary, improving reproducibility.

### `Dockerfile`

Two-stage Docker build targeting Linux/amd64:

**Stage 1 (builder):** Uses `golang:1.25-alpine`. Copies `go.mod` and `go.sum` first (before source code) to maximize layer cache reuse. Builds with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` and the same optimization flags as `make build`.

**Stage 2 (runtime):** Uses `gcr.io/distroless/static-debian12:nonroot`. This image contains only a minimal Linux base with no shell, no package manager, and no writable filesystem except `/tmp`. The binary is copied from the builder stage. The container runs as `nonroot` (UID 65532), not root.

Port 8847 is exposed. The default command is `--transport http --addr 0.0.0.0:8847`.

A `HEALTHCHECK` runs every 30 seconds using `mcp-banana --healthcheck --addr 127.0.0.1:8847`. The health check has a 5-second timeout, a 10-second startup grace period, and retries 3 times before marking the container unhealthy.

### `docker-compose.yml`

Defines a single service named `mcp-banana`:

- Builds from the local `Dockerfile`
- Binds port 8847 to `127.0.0.1:8847` only (loopback, not publicly exposed)
- Loads environment from `.env`
- Restarts automatically unless explicitly stopped (`restart: unless-stopped`)
- Waits 120 seconds before forcibly killing the container on shutdown (`stop_grace_period: 120s`), allowing the server to drain in-flight requests
- Limited to 768 MB of memory
- Logs captured by Docker's `json-file` driver, capped at 10 MB per file with 3 files of rotation

### `mcp-banana`

The compiled binary, produced by `make build`. Not committed (`.gitignore`).

## Configuration

### `.env.example`

Template for runtime configuration. Copy to `.env` and fill in values. See [setup-and-operations.md](setup-and-operations.md) for the full configuration reference table.

### `.mcp.json`

Project-scoped Claude Code MCP server configuration. Committed to the repository so all team members share the same server definition without sharing credentials:

```json
{
  "mcpServers": {
    "banana": {
      "command": "${MCP_BANANA_BIN:-mcp-banana}",
      "args": ["--transport", "stdio"],
      "type": "stdio"
    }
  }
}
```

Each developer provides their own `GEMINI_API_KEY` via their user-scoped Claude Code config or shell environment. See [claude-code-integration.md](claude-code-integration.md) for details.

## Code Quality

### `.golangci.yml`

golangci-lint v2 configuration. Enables `errcheck`, `govet`, `staticcheck`, `unused`, and `ineffassign`. Disables `fieldalignment` (readability over micro-optimization at this scale).

## CI

### `.github/workflows/ci.yml`

CI workflow on feature branch pushes and PRs to `main`. Steps: lint, format, vet, test (80% coverage), binary build (15 MB limit), Docker build (25 MB limit). Action versions pinned to commit SHAs. See [setup-and-operations.md](setup-and-operations.md) for details.

## Version Control

- **`.gitignore`** — Excludes the compiled binary, `.env` files, IDE directories, OS files, and `coverage.out`.
- **`.gitattributes`** — Enforces LF line endings for `*.go`, `Makefile`, `Dockerfile`, `*.sh`, `*.yml`, `*.yaml` to prevent CI failures from CRLF on Windows.
- **`.dockerignore`** — Excludes `.git/`, `.claude/`, `.env`, docs, and `LICENSE` from the Docker build context to keep images small and avoid copying secrets.
