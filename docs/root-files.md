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

Go module definition and dependency checksums. `go.mod` declares the module path (`github.com/reshinto/mcp-banana`), the Go version requirement (`go 1.26.1`), and all direct and indirect dependencies. `go.sum` records the expected cryptographic hashes of every dependency version to prevent supply chain tampering.

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

**Stage 1 (builder):** Uses `golang:1.24-alpine`. Copies `go.mod` and `go.sum` first (before source code) to maximize layer cache reuse. Builds with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` and the same optimization flags as `make build`.

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

The compiled binary, produced by `make build`. Listed in `.gitignore` and not committed. Rebuild with `make build` after pulling new code.

## Configuration

### `.env.example`

Template for the server's runtime configuration. Copy to `.env` and fill in values before running locally or deploying:

```bash
cp .env.example .env
```

Documents all environment variables with their purpose, default values, and validation constraints. The `.env` file itself is excluded from version control by `.gitignore`.

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

golangci-lint v2 configuration. Enables five linters:

| Linter | Purpose |
|---|---|
| `errcheck` | Flags unchecked error return values |
| `govet` | Runs all Go vet analyzers except `fieldalignment` (which is overly strict for maintainability) |
| `staticcheck` | Advanced static analysis for bugs, performance issues, and API misuse |
| `unused` | Flags unexported symbols that are never referenced |
| `ineffassign` | Flags assignments to variables that are immediately overwritten |

`fieldalignment` is explicitly disabled because optimizing struct field order for memory alignment hurts readability without meaningful benefit at this scale.

## CI

### `.github/workflows/ci.yml`

Continuous integration workflow. Triggers on pushes to `feat/**`, `fix/**`, `chore/**` branches and on pull requests to `main`.

Steps: lint, format check, vet, test with 80% coverage enforcement, binary build, binary size check (15 MB limit), Docker image build, Docker image size check (25 MB limit). All action versions are pinned to commit SHAs for supply chain security.

Deployment to production is done manually via SSH. There is no automated CD pipeline.

## Version Control

### `.gitignore`

Excludes: the compiled `mcp-banana` binary (but not the `cmd/mcp-banana/` directory), `.env` and `.env.local` files, IDE directories (`.vscode/`, `.idea/`), OS files (`.DS_Store`, `Thumbs.db`), `coverage.out`, and `.superpowers`.

### `.gitattributes`

Enforces line endings:

- `*.go`, `Makefile`, `Dockerfile`, `*.sh`, `*.yml`, `*.yaml` files use LF (`eol=lf`) regardless of platform
- `.claude/worktrees*` uses LF

This prevents Windows developers from committing CRLF line endings that would fail the `gofmt` check on CI.

### `.dockerignore`

Excludes files from the Docker build context to keep the image small and avoid copying secrets:

- `.git/` - version control metadata
- `.claude/` - Claude Code configuration
- `.env` - runtime secrets
- `*.md` - documentation
- `LICENSE`
- `docs/` - documentation directory

This means documentation changes do not invalidate the Docker build cache.
