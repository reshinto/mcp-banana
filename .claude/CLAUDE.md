# mcp-banana

Go MCP server wrapping Google's Gemini image generation API for Claude Code.

## General Behavior

- When asked to show command output (git status, test results, etc.), show the RAW output first, then summarize if needed. Never replace raw output with a summary.
- When asked to "continue" prior work, produce a brief plan of what you will do within the first response, then start executing. Do not spend the entire session reading files without producing output.

## Tech Stack

- Go 1.24
- mark3labs/mcp-go (MCP protocol)
- google.golang.org/genai (Gemini API client)
- golang.org/x/time/rate (rate limiting)
- Docker distroless (production image)

## Architecture

Dual transport: stdio (Claude Code integration) + HTTP (standalone / DigitalOcean).

Package layout:
- `cmd/mcp-banana/` — entry point, transport selection
- `internal/config/` — env loading and validation
- `internal/gemini/` — API client + model registry (source of truth: `internal/gemini/registry.go`)
- `internal/policy/` — model recommendation logic
- `internal/security/` — input validation, output sanitization (security boundary)
- `internal/server/` — MCP server setup, middleware
- `internal/tools/` — MCP tool handlers

See `.claude/rules/architecture.md` for full architectural constraints.

## Key Paths

- `cmd/mcp-banana/main.go` — entry point and transport selection
- `internal/gemini/registry.go` — model ID source of truth (all model names defined here)
- `internal/security/` — security boundary; all input/output passes through here
- `.github/workflows/` — CI (feature branches + PRs) and CD (main → DigitalOcean)

## Security

- Secrets must NEVER appear in tool responses, logs, or error messages.
- `genai.APIError` must be unwrapped safely — never expose raw API errors to clients.
- All user input passes through `internal/security/` before reaching the Gemini client.
- All Gemini output passes through `internal/security/` before returning to MCP clients.

## Git Workflow

Branch-per-task mandatory. Check existing feature branches before creating new ones. See `.claude/rules/workflow.md`.

## Testing

CI sequence: `golangci-lint run` → `gofmt -w .` → `go vet ./...` → `go test -race -coverprofile=coverage.out ./...` — fix iteratively until all green. Every new feature or algorithm needs tests. See `.claude/rules/testing.md`.

## Documentation

After any file reorganization or code generation task, ALWAYS update all related documentation (README, any doc references) in the same pass. Do not wait for the user to remind you.
