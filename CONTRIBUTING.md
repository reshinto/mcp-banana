# Contributing

## Setup

See [Setup and Operations](docs/setup-and-operations.md) for prerequisites and local setup steps.

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
| `make quality-gate` | Run the full CI sequence: lint → fmt-check → vet → test |
| `make rotate-token` | Generate a new auth token and print rotation instructions |
| `make clean` | Remove the binary and coverage.out |

## Branch Workflow

Every task starts on a new branch from `main`. This is mandatory.

```bash
git checkout main && git pull
git checkout -b feat/short-description   # or fix/ or chore/
```

Branch naming: `feat/<description>`, `fix/<description>`, `chore/<description>`. Never commit directly to `main`.

Do not reuse an existing feature branch for a different task. Check existing branches with `git branch -a` before creating a new one.

## Quality Gate

Run this before every `git add` / `git commit`:

```bash
make quality-gate
```

This runs lint, format check, vet, and tests in order. All steps must pass. Do not skip any step or use `--no-verify`.

## Pull Request Process

1. Push your branch: `git push -u origin feat/your-branch`
2. Open a PR targeting `main` — do not leave pushed branches without a PR
3. PR title: under 70 characters, imperative mood ("Add image editing support", not "Added...")
4. PR body: summary bullet points and a test plan checklist
5. Ensure all CI checks pass before requesting review

Every feature or bug fix needs tests. See [docs/testing.md](docs/testing.md) for patterns and the 80% coverage threshold.

## Coding Standards

### Naming

No single-character variable names. Use full, descriptive names everywhere — in tests, temporary variables, and loop iterators:

```go
// correct
for elementIndex, item := range items { ... }

// wrong — banned by coding standards
for i, v := range items { ... }
```

Allowed standard Go abbreviations (do not expand these):

| Abbreviation | Meaning |
|---|---|
| `err` | error value |
| `ctx` | context.Context |
| `req` | request |
| `resp` | response |
| `cfg` | config/configuration struct |
| `srv` | server |
| `test` | `*testing.T` parameter in test functions |

All other abbreviations must use full words. Examples of banned forms: `mw`, `msg`, `sig`, `idx`, `num`, `val`, `buf`, `tmp`, `fn`, `cb`, `ch`, `opts`, `addr`.

Use context-specific error names when multiple errors are in scope: `loadError`, `validationError`, `parseError`.

### Test Parameter Convention

Test functions use `test *testing.T`, not `t *testing.T`:

```go
func TestSomething(test *testing.T) {
    // ...
}
```

### Imports

Group imports with a blank line between each group:

1. Standard library
2. Third-party packages
3. Internal packages (`github.com/reshinto/mcp-banana/internal/...`)

When two packages share the same base name, use an import alias:

```go
import (
    "github.com/mark3labs/mcp-go/server"
    internalserver "github.com/reshinto/mcp-banana/internal/server"
)
```

### File Length

Keep source files focused and under 500 lines. If a file grows beyond this, split it by responsibility.

### Error Handling

Always check errors. The `errcheck` linter will flag unchecked error returns.

Use `fmt.Errorf("context: %w", err)` to wrap errors with context while preserving the original for `errors.As`/`errors.Is` unwrapping.

### Comments

Every exported function and type must have a Go doc comment. Security-critical code must include a `// SECURITY:` annotation explaining the invariant being enforced.

### Security Constraints

- Never include secrets in log output, error messages, or tool responses. Register secrets with `security.RegisterSecret()` at startup.
- Never expose `GeminiID` values to Claude Code. Use `SafeModelInfo` for all external responses.
- Always validate user input through the `internal/security/` package before forwarding to the Gemini client.
- Map all Gemini errors through `gemini.MapError()` — never forward raw SDK error text.

## Adding a New Feature

1. Identify which `internal/` package the feature belongs to.
2. Implement the feature in that package (logic + types + `_test.go` co-located).
3. If the feature introduces a new MCP tool, register it in `internal/tools/` and wire it in `internal/server/server.go`.
4. If the feature uses a new model, add the model ID to `internal/gemini/registry.go` first.
5. Update README and relevant docs in the same pass — do not defer doc updates.

## CI Requirements

The CI pipeline runs on feature branch pushes and PRs:

1. `golangci-lint run`
2. `gofmt -w .`
3. `go vet ./...`
4. `go test -race -coverprofile=coverage.out ./...` with 80% line coverage enforcement
5. Binary build: `go build ./cmd/mcp-banana/`

All checks must pass before a PR can be merged.

## OAuth Development Setup

OAuth requires real provider credentials for end-to-end testing. For local development that does not involve OAuth, use bearer token authentication — it works without a domain, TLS certificate, or provider credentials.

To test the OAuth flow locally:

1. Register a test application with at least one provider. See [docs/authentication.md](docs/authentication.md) for provider console links.
2. Set the redirect URI to `http://localhost:8847/callback` — most providers allow `localhost` callbacks.
3. Set `OAUTH_BASE_URL=http://localhost:8847` in your local `.env`. TLS is not required for `localhost`.
4. Run the server with `make run-http` and open `http://localhost:8847/authorize` to test the flow.

Do not commit provider credentials. Keep them in your local `.env`, which is excluded by `.gitignore`.

## Further Reading

- [docs/architecture.md](docs/architecture.md) — System design, package layout, security boundaries
- [docs/testing.md](docs/testing.md) — Testing patterns, inventory, and coverage requirements
- [docs/go-guide.md](docs/go-guide.md) — Go language concepts used in this project
- [docs/security.md](docs/security.md) — Threat model and security controls
- [docs/models.md](docs/models.md) — Model aliases and sentinel ID verification procedure
- [docs/root-files.md](docs/root-files.md) — Description of every root-level file and directory
