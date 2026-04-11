# Root Files Reference

Every file and directory at the project root — what it is, why it exists, and how to interact with it.

---

## Quick Reference

| Item | Purpose | Command / note |
|---|---|---|
| `cmd/` | Binary entry point | `make build` |
| `internal/` | All application packages | — |
| `go.mod` / `go.sum` | Module definition and checksums | `go mod tidy` |
| `Makefile` | Developer task runner | `make quality-gate` |
| `Dockerfile` | Two-stage production image | `docker build -t mcp-banana .` |
| `docker-compose.yml` | Dev Docker stack (loopback) | `./scripts/run-docker-dev.sh` |
| `docker-compose.prod.yml` | Production overlay (public + TLS) | `./scripts/run-docker-prod.sh` |
| `scripts/` | Helper shell scripts | see Scripts section |
| `.env.example` | Runtime config template | `cp .env.example .env` |
| `.mcp.json` | Claude Code MCP server registry | add server entries here |
| `.claude/` | Claude Code AI assistant config | — |
| `.golangci.yml` | Linter configuration | `make lint` |
| `.github/workflows/ci.yml` | CI pipeline | triggered on PRs to `main` |
| `.gitignore` | Version control exclusions | — |
| `.gitattributes` | Line-ending normalization | — |
| `.dockerignore` | Docker build-context exclusions | — |
| `docs/` | All project documentation | — |
| `README.md` | Project overview and quick start | — |
| `CONTRIBUTING.md` | Developer workflow and standards | — |
| `LICENSE` | MIT license, copyright Terence 2026 | — |

---

## Source Code

### `cmd/`

**What it is:** The entry point for the `mcp-banana` binary. Contains one package at `cmd/mcp-banana/main.go`.

**Purpose:** Wires all dependencies together and selects the transport. No business logic lives here — `cmd/` only calls into `internal/` packages. The `run()` function is extracted from `main()` so it can be fully tested.

**Key startup sequence in `main.go`:**
1. Parse CLI flags: `--transport` (`stdio` or `http`), `--addr`, `--healthcheck`, `--version`
2. Load and validate configuration via `internal/config`
3. Validate the model registry via `internal/gemini`
4. Register all secrets with the sanitizer (`internal/security.RegisterSecret`)
5. Build the Gemini client and client cache
6. Construct the MCP server via `internal/server`
7. Start stdio (`server.ServeStdio`) or HTTP (`net/http.Server` with graceful shutdown)

**How to use it:**
```bash
make build                              # compile to ./mcp-banana
./mcp-banana --transport stdio          # Claude Code direct integration
./mcp-banana --transport http --addr 0.0.0.0:8847  # standalone HTTP
./mcp-banana --version
./mcp-banana --healthcheck --addr 127.0.0.1:8847
```

---

### `internal/`

**What it is:** All application code. Go's `internal/` convention prevents these packages from being imported by code outside this module.

**Purpose:** Isolates each domain behind a clean package boundary. The security boundary (`internal/security/`) sits between all user input and the Gemini API.

| Package | Responsibility |
|---|---|
| `config/` | Load environment variables into a `Config` struct; validate required fields and constraints |
| `gemini/` | Gemini API client, model registry (`registry.go`), error mapping, client cache, per-request key support |
| `oauth/` | OAuth 2.1 authorization: PKCE flow, dynamic client registration, token store, Google/GitHub/Apple providers |
| `policy/` | Model recommendation logic — keyword matching and priority selection for `recommend_model` |
| `security/` | Input validation and output sanitization; secret registration and redaction from logs/responses |
| `server/` | HTTP routing, middleware chain (bearer auth, rate limiting, concurrency, body size), MCP server construction |
| `tools/` | MCP tool handler factories: `generate_image`, `edit_image`, `list_models`, `recommend_model` |

**How to use it:** Import packages by their full path, e.g. `github.com/reshinto/mcp-banana/internal/gemini`. Never import across packages by accessing unexported fields — use the public interface.

---

### `go.mod` and `go.sum`

**What they are:** `go.mod` defines the module path (`github.com/reshinto/mcp-banana`), Go version (1.25), and direct dependencies. `go.sum` records the cryptographic hash of every dependency version.

**Purpose:** `go.mod` pins direct dependency versions. `go.sum` prevents supply-chain tampering — Go refuses to use a dependency whose hash does not match.

**Direct dependencies:**

| Module | Purpose |
|---|---|
| `github.com/mark3labs/mcp-go` | MCP protocol server (stdio and HTTP transports) |
| `google.golang.org/genai` | Gemini API client library |
| `golang.org/x/time` | Token-bucket rate limiter (`rate.Limiter`) |

**How to use it:**
```bash
go mod tidy        # remove unused deps and add missing ones
go mod download    # fetch all deps into the module cache
go get <module>@<version>   # add or upgrade a dependency
```

---

## Configuration

### `.env.example`

**What it is:** A template listing every environment variable the server reads, with description, default, format notes, and security guidance. No actual secrets — all values are blank.

**Purpose:** Documents the full configuration surface in one place. Serves as the authoritative reference for operators setting up `.env`.

**How to use it:**
```bash
cp .env.example .env
# Edit .env and fill in values
```

**Variable reference:**

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `MCP_DOMAIN` | Production only | — | Domain name for TLS cert path and OAuth redirect URLs |
| `MCP_CREDENTIALS_FILE` | Optional | — | Path to a JSON file mapping bearer tokens and OAuth identities to Gemini API keys; hot-reloaded on every request |
| `MCP_LOG_LEVEL` | Optional | `info` | Logging verbosity: `debug`, `info`, `warn`, `error` |
| `MCP_RATE_LIMIT` | Optional | `30` | Max requests per minute (all models) |
| `MCP_GLOBAL_CONCURRENCY` | Optional | `8` | Max simultaneous Gemini API calls |
| `MCP_PRO_CONCURRENCY` | Optional | `3` | Max simultaneous calls to the Pro model (must be ≤ `MCP_GLOBAL_CONCURRENCY`) |
| `MCP_MAX_IMAGE_BYTES` | Optional | `4194304` | Max decoded image size in bytes for `edit_image` |
| `MCP_REQUEST_TIMEOUT_SECS` | Optional | `120` | Timeout per Gemini API call in seconds |
| `OAUTH_BASE_URL` | Optional | auto | Base URL for OAuth endpoints; auto-populated from `MCP_DOMAIN` by the prod script |
| `OAUTH_GOOGLE_CLIENT_ID` / `_SECRET` | Optional | — | Google OAuth credentials |
| `OAUTH_GITHUB_CLIENT_ID` / `_SECRET` | Optional | — | GitHub OAuth credentials |
| `OAUTH_APPLE_CLIENT_ID` / `_SECRET` | Optional | — | Apple Sign-In credentials |
| `MCP_TLS_CERT_FILE` | Optional | — | Path to PEM certificate; enables HTTPS when both TLS vars are set |
| `MCP_TLS_KEY_FILE` | Optional | — | Path to PEM private key |

---

### `.mcp.json`

**What it is:** Project-scoped Claude Code MCP server registry. Currently contains an empty `mcpServers` object.

**Purpose:** When populated, this file allows Claude Code to discover the MCP server automatically for all contributors cloning the repository, without per-user setup.

**How to use it:** Add a server entry pointing at a running instance:
```json
{
  "mcpServers": {
    "banana": {
      "type": "http",
      "url": "http://localhost:8847/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

---

### `.claude/`

**What it is:** Claude Code project configuration directory. Contains instructions, rules, hooks, agent definitions, and skills for the AI assistant.

**Purpose:** Shapes how Claude Code behaves in this repository — enforcing coding conventions, security rules, branch strategy, and workflow steps automatically.

**Contents:**

| Item | What it contains |
|---|---|
| `CLAUDE.md` | Primary instructions: tech stack, architecture overview, key paths, security constraints, and general behavior rules |
| `rules/architecture.md` | Package layout rules, security boundary constraints, model registry invariants |
| `rules/coding-standards.md` | Naming conventions, import ordering, file size limits, comment requirements |
| `rules/docker-ci-cd.md` | Dockerfile conventions, CI/CD pipeline steps, image tagging policy |
| `rules/docs.md` | Documentation update triggers, style guidelines |
| `rules/testing.md` | Test runner, coverage thresholds (80%), test naming, what to test |
| `rules/token-efficiency.md` | Instructions for concise, efficient responses |
| `rules/workflow.md` | Mandatory branch-per-task strategy, development flow, PR requirements |
| `hooks/block-ai-attribution.sh` | PreToolUse: blocks `Co-Authored-By` AI attribution in commit messages |
| `hooks/block-main-branch-commits.sh` | PreToolUse: prevents direct commits and pushes to `main` |
| `hooks/enforce-branch-naming.sh` | PreToolUse: validates `<type>/<description>` branch naming on checkout |
| `hooks/enforce-naming-convention.sh` | PreToolUse: validates coding standard naming rules before commit |
| `hooks/enforce-file-size.sh` | PreToolUse: flags source files over 500 lines before commit |
| `hooks/auto-pr-after-push.sh` | PostToolUse: opens a PR automatically after a branch is pushed |
| `hooks/session-start-branch-check.sh` | SessionStart: warns if the session begins on `main` |
| `hooks/auto-plugin-mode.sh` | SessionStart: activates plugin mode if applicable |
| `hooks/session-end-unified-gate.sh` | Stop: runs the quality gate at session end |
| `hooks/session-end-claude-system-check.sh` | Stop: verifies `.claude/` system integrity |
| `agents/` | Specialized agent definitions (tech lead, QA tester, code reviewer, etc.) |
| `settings.json` | Hook bindings, tool permissions, enabled plugins |
| `settings.local.json` | Local overrides — excluded from version control |

---

## Docker

### `Dockerfile`

**What it is:** Two-stage Docker build producing a hardened Linux/amd64 production image.

**Purpose:** Creates a minimal, secure container image. The distroless runtime has no shell, no package manager, and no writable filesystem beyond `/tmp`, minimizing the attack surface.

**Stage 1 — builder (`golang:1.25-alpine`):**
- Copies `go.mod` and `go.sum` before source files, so dependency downloads are cached independently of source changes
- Builds with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 -ldflags="-s -w" -trimpath` — statically linked, debug info stripped, no absolute file paths embedded

**Stage 2 — runner (`gcr.io/distroless/static-debian12:nonroot`):**
- Copies only the compiled binary from the builder stage
- Runs as `nonroot` (UID 65532) — never root
- Exposes port 8847
- Default command: `--transport http --addr 0.0.0.0:8847`
- Health check: polls `--healthcheck --addr 127.0.0.1:8847` every 30s, 5s timeout, 10s startup grace, 3 retries

**How to use it:**
```bash
docker build -t mcp-banana .
docker run -p 8847:8847 --env-file .env mcp-banana
```

---

### `docker-compose.yml`

**What it is:** Development Docker Compose stack defining a single `mcp-banana` service.

**Purpose:** Provides a one-command local development environment. Binds only to the loopback interface so the server is not publicly accessible without an SSH tunnel.

**Key fields:**

| Field | Value | Why |
|---|---|---|
| `build: .` | Build from local `Dockerfile` | Always use current source |
| `ports: 127.0.0.1:8847:8847` | Loopback only | Not publicly exposed in dev |
| `env_file: .env` | Load all config from `.env` | Keeps secrets out of `docker-compose.yml` |
| `restart: unless-stopped` | Auto-restart on crash or reboot | Keeps the service running |
| `stop_grace_period: 120s` | Wait 120s before force-kill | Allows in-flight requests to complete |
| `mem_limit: 768m` | Cap memory at 768 MB | Protects the host from runaway usage |
| `logging.options.max-size: 10m` | Rotate logs at 10 MB | Prevents unbounded disk usage |
| `logging.options.max-file: 3` | Keep 3 log files | ~30 MB total log retention |

**How to use it:**
```bash
./scripts/run-docker-dev.sh          # start (builds first)
docker compose logs -f               # stream logs
docker compose down                  # stop and remove containers
```

---

### `docker-compose.prod.yml`

**What it is:** A Docker Compose override file that extends `docker-compose.yml` for production deployment.

**Purpose:** Uses the overlay pattern — it is never used alone. When merged with the base file, it overrides only the fields that differ in production: public interface binding and TLS certificate mounting.

**Fields:**

| Field | Value | Why |
|---|---|---|
| `ports: 0.0.0.0:8847:8847` | All interfaces | Production server must be publicly reachable |
| `volumes: /etc/letsencrypt/live/${MCP_DOMAIN}:/certs:ro` | Mount Let's Encrypt certs read-only | TLS without copying certs into the image |

**Overlay pattern:** Docker Compose merges the two files at startup. Fields in `docker-compose.prod.yml` override matching fields from `docker-compose.yml`; all other fields from the base file remain.

**How to use it:**
```bash
./scripts/run-docker-prod.sh         # start (handles cert generation and validation)
# Manual equivalent:
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build
```

---

## Scripts

All scripts in `scripts/` are self-contained bash scripts. They `cd` to the project root automatically and validate prerequisites before doing anything destructive.

| Script | What it does | Usage |
|---|---|---|
| `run-local.sh` | Loads `.env`, runs `make build`, starts the binary in stdio mode | `./scripts/run-local.sh` |
| `run-docker-dev.sh` | Validates `.env` exists, runs `docker compose up -d --build`, prints connection info | `./scripts/run-docker-dev.sh` |
| `run-docker-prod.sh` | Full production deployment: validates `.env` and `MCP_DOMAIN`, auto-populates `OAUTH_BASE_URL`/TLS paths, creates `credentials.json` if missing, runs certbot if certs are missing, starts the production stack, waits for `/healthz` to return healthy | `./scripts/run-docker-prod.sh` |

`run-docker-prod.sh` will not start Docker unless all prerequisites are satisfied. If anything fails, it prints a specific error and exits non-zero before modifying state.

---

## Build and Quality

### `Makefile`

**What it is:** The primary developer task runner. All targets are declared `.PHONY`.

**Purpose:** Provides short, memorable commands for the full development lifecycle and enforces the CI sequence locally.

| Target | Command | When to use |
|---|---|---|
| `build` | `CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o mcp-banana ./cmd/mcp-banana/` | Compile the binary |
| `test` | `go test -coverprofile=coverage.out -race ./... -v` | Run all tests with race detection |
| `lint` | `golangci-lint run` | Run the full linter suite |
| `fmt` | `gofmt -w .` | Format all Go source files in place |
| `fmt-check` | Fails if `gofmt -l .` outputs anything | Formatting check without writing (used in CI) |
| `vet` | `go vet ./...` | Static analysis |
| `run-stdio` | Builds then `./mcp-banana --transport stdio` | Test stdio mode locally |
| `run-http` | Builds then `./mcp-banana --transport http --addr 0.0.0.0:8847` | Test HTTP mode locally |
| `clean` | `rm -f mcp-banana coverage.out` | Remove build artifacts |
| `rotate-token` | Generates `openssl rand -hex 32` and prints step-by-step rotation instructions | Rotate the bearer token |
| `quality-gate` | `lint` → `fmt-check` → `vet` → `test` in order | Run before every `git commit` |

**How to use it:** Run `make quality-gate` before staging any commit. All four steps must pass.

---

### `.golangci.yml`

**What it is:** golangci-lint v2 configuration. Specifies which linters are active and their settings.

**Purpose:** Enforces code quality checks beyond what `go vet` and `gofmt` provide. Configured to match the project's conventions without false positives.

**Enabled linters:**

| Linter | What it catches |
|---|---|
| `errcheck` | Unchecked error return values |
| `govet` | All `go vet` checks except `fieldalignment` (struct layout optimization — intentionally excluded) |
| `staticcheck` | Advanced static analysis: unused results, deprecated APIs, incorrect string formatting |
| `unused` | Exported symbols that are never referenced |
| `ineffassign` | Assignments whose values are never read |

`stylecheck` and `revive` are intentionally not enabled. Do not add them.

**How to use it:**
```bash
make lint         # run via Make
golangci-lint run # run directly
```

---

## CI/CD

### `.github/workflows/ci.yml`

**What it is:** GitHub Actions CI workflow.

**Purpose:** Runs automated quality and safety checks on every PR targeting `main`. Blocks merging if any step fails.

**Trigger:** `pull_request` on branches targeting `main`.

**Steps in order:**

| Step | Command | What it validates |
|---|---|---|
| Lint | `golangci-lint run` | Linter rules from `.golangci.yml` |
| Format check | `gofmt -l .` | All files are formatted |
| Type check | `go vet ./...` | No static analysis errors |
| Tests | `go test -race -coverprofile=coverage.out ./...` | No failures; race detector enabled |
| Coverage gate | `awk` check on `go tool cover` output | Total line coverage ≥ 80% |
| Coverage upload | `actions/upload-artifact` | Stores `coverage.out` for 7 days |
| Binary build | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ...` | Binary compiles for Linux |
| Binary size check | `stat` + shell comparison | Binary is ≤ 15 MB |
| Docker build | `docker build -t mcp-banana:ci .` | Image builds successfully |
| Docker size check | `docker image inspect` | Image is ≤ 25 MB |

All action versions are pinned to commit SHAs to prevent supply-chain attacks.

---

## Version Control

### `.gitignore`

**What it is:** Git exclusion list for files that should never be committed.

**Purpose:** Prevents accidental commits of secrets, build artifacts, and local configuration.

**Excluded categories:**

| Pattern | What it excludes |
|---|---|
| `mcp-banana` (binary) | Compiled binary (but `!cmd/mcp-banana/` keeps source) |
| `.env`, `.env.local`, `.env.*.local` | Local secrets and configuration |
| `.vscode/`, `.idea/`, `*.swp`, `*.suo` | IDE and editor state |
| `.DS_Store`, `Thumbs.db` | OS-generated metadata |
| `coverage.out`, `coverage.html`, `cov.out`, `cov_*.out` | Test coverage artifacts |
| `.claude/settings.local.json` | Per-developer Claude Code overrides |
| `.superpowers` | Claude Code superpowers runtime state |
| `.claude/worktrees` | Git worktree directories created by Claude Code |

---

### `.gitattributes`

**What it is:** Git attribute rules that control how Git handles file content.

**Purpose:** Enforces LF line endings on all text files, preventing CI failures caused by CRLF characters introduced by Windows editors or Git's `core.autocrlf` setting.

**Rules:**

| Pattern | Attribute |
|---|---|
| `*` | `text=auto eol=lf` — normalize all text files to LF |
| `*.go` | `text eol=lf` — explicit LF for Go source |
| `Makefile` | `text eol=lf` — Makefiles require LF for tab indentation |
| `Dockerfile` | `text eol=lf` |
| `*.sh` | `text eol=lf` — shell scripts require LF |
| `*.yml` / `*.yaml` | `text eol=lf` |

---

### `.dockerignore`

**What it is:** Exclusion list for the Docker build context.

**Purpose:** Reduces the size of the context sent to the Docker daemon and prevents sensitive or irrelevant files from being accidentally copied into the image.

**Excluded items:**

| Pattern | Why excluded |
|---|---|
| `.git` | Version control history — not needed in the image |
| `.claude` | AI assistant configuration — not needed in the image |
| `.env` | Local secrets — must never end up in the image |
| `*.md` | Documentation — not needed in the image |
| `LICENSE` | License text — not needed in the image |
| `docs/` | Documentation directory — not needed in the image |

---

## Documentation

### `docs/`

All project documentation. The `assets/` subdirectory contains images (screenshots, demo GIF). The `diagrams/` subdirectory contains architecture diagrams.

| File | Contents |
|---|---|
| `architecture.md` | System design, package layout, data flow, security boundary diagram |
| `authentication.md` | Bearer token setup, OAuth 2.1 flow, multi-user token file |
| `claude-code-integration.md` | Full Claude Code setup: stdio, HTTP, SSH tunnel, troubleshooting |
| `go-guide.md` | Go language concepts used in this codebase with annotated examples |
| `models.md` | Model aliases, latency ranges, use cases, sentinel ID verification procedure |
| `root-files.md` | This file |
| `security.md` | Threat model, security controls, secret handling, input/output sanitization |
| `setup-and-operations.md` | Prerequisites, local dev setup, Docker deployment, environment variable reference |
| `testing.md` | Test runner, coverage thresholds, test patterns, security test requirements |
| `tools-reference.md` | Full JSON schema for all four MCP tools (`generate_image`, `edit_image`, `list_models`, `recommend_model`) |
| `troubleshooting.md` | Common failure modes, error messages, and step-by-step diagnostics |

---

### `README.md`

**What it is:** The project's public-facing overview document.

**Purpose:** First document a new user reads. Covers what the project is, what it does, and how to get started in under five minutes.

**Contents:** Demo screenshot, four-tool overview, quick-start (clone → configure → start → register with Claude Code → generate an image), architecture diagram, links to detailed docs.

---

### `CONTRIBUTING.md`

**What it is:** Developer onboarding and contribution guide.

**Purpose:** Documents everything a developer needs to contribute: setup steps, all `make` targets, branch naming rules, quality gate requirements, coding standards, PR process, CI requirements, OAuth development setup, and links to further reading.

**Key sections:** Development commands table, branch workflow, quality gate, naming conventions (including the ban on single-character variable names and the `test *testing.T` parameter convention), import grouping, error handling, security constraints, adding a new feature, OAuth local testing.

---

## License

### `LICENSE`

MIT License. Copyright (c) 2026 Terence. Free to use, copy, modify, merge, publish, distribute, sublicense, and sell. The copyright notice and license text must be included in all copies.
