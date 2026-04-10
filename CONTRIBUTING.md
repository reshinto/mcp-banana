# Contributing

## Prerequisites

| Tool | Version | Required For |
|---|---|---|
| Go | 1.24 or later | Building and testing |
| golangci-lint | v2.1.6 or later | Linting (`make lint`) |
| Docker | Any recent version | Container builds and testing |
| OpenSSL | Any | Generating auth tokens |

Install golangci-lint:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.1.6
```

## Getting Started

```bash
git clone https://github.com/reshinto/mcp-banana.git
cd mcp-banana
go mod download
make build
```

Set required environment variables:

```bash
export GEMINI_API_KEY="AIza..."
```

Before the server can start, verify the model IDs in `internal/gemini/registry.go`. See [docs/models.md](docs/models.md) for the verification procedure.

## Development Commands

| Command | Description |
|---|---|
| `make build` | Compile the binary to `./mcp-banana` |
| `make test` | Run all tests with race detection and coverage |
| `make lint` | Run golangci-lint |
| `make fmt` | Format all Go source files with gofmt |
| `make fmt-check` | Check formatting without writing changes (used in CI) |
| `make vet` | Run `go vet ./...` static analysis |
| `make run-stdio` | Build and run in stdio mode |
| `make run-http` | Build and run in HTTP mode on `0.0.0.0:8847` |
| `make quality-gate` | Run the full CI sequence: lint, fmt-check, vet, test |
| `make rotate-token` | Generate a new auth token and print rotation instructions |
| `make clean` | Remove the binary and coverage.out |

## Coding Standards

### Naming

- No single-character variable names. Use `elementIndex` instead of `i`, `currentNode` instead of `n`.
- No variable names composed of combined single characters (`ij`, `xy`).
- Use meaningful names in tests and for temporary variables as well.
- Common abbreviations are acceptable when their meaning is clear and consistent with the rest of the codebase (see glossary below).

### Imports

Group imports with a blank line between groups:

1. Standard library
2. Third-party packages
3. Internal packages (`github.com/reshinto/mcp-banana/internal/...`)

When two packages have the same base name, use an import alias:

```go
import (
    "github.com/mark3labs/mcp-go/server"
    internalserver "github.com/reshinto/mcp-banana/internal/server"
)
```

### File Length

Keep source files focused. Files should generally stay under 500 lines. If a file is growing larger, consider splitting it by responsibility.

### Error Handling

Always check errors. The `errcheck` linter will flag unchecked error returns. Return errors to the caller rather than logging and continuing, unless the error is truly non-fatal and the decision to continue is explicit.

Use `fmt.Errorf("context: %w", err)` to wrap errors with context while preserving the original error for `errors.As`/`errors.Is` unwrapping.

### Security Constraints

- Never include secrets in log output, error messages, or tool responses. Register secrets with `security.RegisterSecret()` at startup.
- Never expose `GeminiID` values to Claude Code. Use `SafeModelInfo` for all external responses.
- Always validate user input through the `security` package before forwarding to the Gemini client.
- Map all Gemini errors through `gemini.MapError()` - never forward raw error text.

## Branch Workflow

Every task starts on a new branch:

```bash
git checkout main && git pull
git checkout -b feat/short-description   # or fix/ or chore/
```

Branch naming:
- `feat/<description>` - new feature
- `fix/<description>` - bug fix
- `chore/<description>` - maintenance, dependency update, tooling

Never commit directly to `main`.

## Quality Gate

Run this before every commit:

```bash
make quality-gate
```

This runs lint, format check, vet, and tests in order. All steps must pass. Fix issues iteratively and re-run until all steps are green.

The CI pipeline runs the same sequence. A failing quality gate will block the PR from being merged.

## Pull Request Process

1. Push your branch: `git push -u origin feat/your-branch`
2. Open a PR targeting `main`
3. The PR title must be under 70 characters and use imperative mood ("Add image editing support", not "Added..." or "Adding...")
4. Ensure all CI checks pass
5. Request review

Every feature or bug fix needs tests. See [docs/testing.md](docs/testing.md) for patterns and the coverage threshold.

## Go Glossary

Common abbreviations used in this codebase:

| Abbreviation | Meaning | Explanation |
|---|---|---|
| `err` | error | Holds the error returned by the previous operation. In Go, functions return errors as values, not exceptions, so error handling is explicit. |
| `ctx` | context | Carries request-scoped deadlines, cancellation signals, and metadata across API boundaries. Essential for managing timeouts and coordinating shutdown. |
| `req` | request | The incoming tool call or HTTP request. Contains arguments, headers, and body. |
| `resp` | response | The HTTP or API response received from an upstream service. Contains status code, headers, and body. |
| `cfg` | config | The parsed application configuration loaded from environment variables at startup. |
| `srv` | server | The HTTP server instance or MCP server. In this project: `*http.Server` or `*mcpserver.MCPServer`. |
| `test` | test | The Go test runner (`*testing.T`) that provides logging and failure reporting. Used in all unit tests. |

## Further Reading

- [docs/architecture.md](docs/architecture.md) - System design, package layout, data flow
- [docs/testing.md](docs/testing.md) - Testing patterns, inventory, and coverage requirements
- [docs/go-guide.md](docs/go-guide.md) - Go language concepts used in this project
- [docs/security.md](docs/security.md) - Threat model and security controls
- [docs/models.md](docs/models.md) - Model aliases and verification procedure
