# mcp-banana Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. **Execute tasks SEQUENTIALLY to optimize token usage.** Do NOT parallelize tasks.

**Goal:** Build a production-ready Go MCP server that wraps Google's Nano Banana image generation API for Claude Code, with server-side secret isolation, dual transport (stdio for local development, HTTP for remote deployment), and Docker deployment.

**Architecture:** Single Go binary using `mark3labs/mcp-go` for MCP protocol and `google.golang.org/genai` for Gemini API. 4 MCP tools exposed (generate_image, edit_image, list_models, recommend_model). Secrets stay server-side. Model aliases map to Gemini IDs internally. Handlers depend on a `GeminiService` interface for testability.

**Tech Stack:** Go 1.24, `github.com/mark3labs/mcp-go`, `google.golang.org/genai`, `golang.org/x/time/rate`, `log/slog`, Docker with distroless base.

**Design spec:** `docs/superpowers/specs/2026-04-10-mcp-banana-design.md`

---

## Model Guidance for Each Task

| Task Type | Recommended Model | Rationale |
|---|---|---|
| Scaffolding (mkdir, go mod init, boilerplate files) | **Haiku** | Mechanical, no creativity needed |
| Config/constants/registry (simple structs, maps) | **Haiku** | Pattern-following, low complexity |
| Security code (validation, sanitization, auth) | **Sonnet** | Needs careful reasoning about edge cases |
| Gemini client wrapper + error handling | **Sonnet** | SDK integration, error taxonomy mapping |
| MCP tool handlers | **Sonnet** | Business logic, input/output design |
| Policy/recommendation algorithm | **Sonnet** | Logic design with keyword matching |
| Tests (all types) | **Sonnet** | Needs to reason about edge cases and adversarial inputs |
| Middleware (rate limiting, concurrency) | **Sonnet** | Concurrency patterns, correctness critical |
| Main entry point + graceful shutdown | **Sonnet** | Signal handling, transport wiring |
| Dockerfile + docker-compose | **Haiku** | Template-following, well-documented patterns |
| README + documentation | **Haiku** | Prose generation from known structure |
| Quality gate (lint, vet, test) | **Haiku** | Running commands, fixing mechanical issues |

**Token optimization rules:**
- Run tasks sequentially, never in parallel
- Read only the files you need for the current task
- Do not re-read the full plan between tasks -- use the checklist below
- Commit after each task completes (small, focused commits)
- Run the local quality gate (Task 14) after all application code tasks (0-13) and before CI/CD and integration tasks (15-18)
- **Plugin sync is a one-time pre-implementation cleanup only (Task 0 Step 8).** After Task 0, `settings.json` contains the canonical plugin list and all plugin entries remain enabled. Future plugin changes happen only in `settings.local.json`.
- **Never commit `.claude/settings.local.json`**. It is personal/local. Most tasks in this plan use specific `git add <files>` which naturally excludes it. When using broad staging (`git add -A` or `git add .`), always follow with `git reset .claude/settings.local.json 2>/dev/null || true` before committing.

**Naming convention (MANDATORY -- enforced by code review; advisory pre-commit hook catches common violations only):**
- No single-character variable names (no `i`, `j`, `k`, `n`, `x`, `y`, `z`, `t`, `s`, `w`, `r`, `m`)
- No abbreviations that are not complete words (no `mw`, `msg`, `sig`, `idx`, `num`, `val`, `buf`, `tmp`, `fn`, `cb`, `op`, `ch`, `opts`, `addr`)
- **Allowed standard Go abbreviations** (idiomatic Go, permitted without inline comments): `err`, `ctx`, `req`, `resp`, `cfg`, `srv`, `test` (for `*testing.T`). These are explained in the Go glossary in the README for contributors unfamiliar with Go.
- All other non-standard abbreviations are forbidden. Variables must be complete, meaningful words or phrases: `loadError`, `httpServer`, `middleware`, `message`, `healthResponse`, `receivedSignal`, `index`, `generateOptions`
- This applies to ALL code: production code, test code, loop iterators, temporary variables, function parameters

**Modularity rules (MANDATORY -- enforced by code review, with advisory pre-commit hook):**
- Maximum 500 lines per file. Files that grow beyond this must be split into focused sub-files.
- One responsibility per function. Each function should do exactly one thing and be short enough to understand at a glance.
- One objective per struct/type. Structs should represent a single concept, not bundle unrelated concerns.
- DRY principle. Extract shared logic into reusable functions. Do not duplicate code across files.
- Centralize public-facing and cross-file string literals as named constants. This includes: error codes returned to clients (e.g., `ErrContentPolicy`), their safe messages, and MIME types referenced in multiple files. String literals that appear only within a single package and whose meaning is clear from context do not need extraction (e.g., env var names in config loading, model aliases in the registry map, tool names in tool registration, protocol paths, log messages, test assertions, struct tags).
- If a file or function is growing large, that is a signal it needs to be decomposed.

**Code comments and documentation (MANDATORY):**
- Every file must have a package-level comment explaining the file's purpose and how it fits in the architecture
- Every exported function must have a comment explaining what it does, its parameters, and return values
- Every exported type/struct must have a comment explaining its purpose and invariants
- Comments should be written for a new programmer who has never seen Go or this codebase
- Focus on **why** (purpose, intent, design decisions) not just **what** (which is visible from the code)
- Security-critical code must have explicit SECURITY comments about what is safe and what is dangerous

**Logging safety rule (MANDATORY):**
- Raw upstream Gemini errors must NEVER be logged on request paths. Use typed safe error codes/messages only.
- Output sanitization (`SanitizeString`) is defense-in-depth, NOT the primary protection. The primary protection is: never pass raw SDK error text into any string that could reach logs, responses, or health checks.

**Model ID status:**
- This plan uses `VERIFY_MODEL_ID_BEFORE_RELEASE` sentinels in the model registry. The plan is **implementation-ready but release-blocked** until real Gemini model IDs are verified against the live API. Before the first release, verify IDs at https://ai.google.dev/gemini-api/docs/models and update `internal/gemini/registry.go`.

---

## Pre-Implementation Checklist

**MANDATORY first steps before ANY code implementation:**

- [ ] **Step A: Stash current changes** -- the docs/specs/plans files are uncommitted and must be preserved:
```bash
cd /Users/springfield/dev/mcp-banana
git stash push -u -m "pre-mcp-server-plan"
```
- [ ] **Step B: Create feature branch from main:**
```bash
git checkout main && git pull
git checkout -b feat/mcp-server
```
- [ ] **Step C: Restore stashed changes onto the new branch:**
```bash
git stash list
git stash apply 'stash^{/pre-mcp-server-plan}'
# If conflicts occur, resolve them before continuing.
# Verify the restored files look correct, then drop the stash:
git stash drop 'stash^{/pre-mcp-server-plan}'
```
- [ ] **Step D: Commit the plan and spec files as the first commit on the branch:**
```bash
git add docs/
git commit -m "docs: add implementation plan and design spec"
```

Then verify:
- [ ] On branch `feat/mcp-server`
- [ ] Working directory is `/Users/springfield/dev/mcp-banana`
- [ ] Plan and spec files are committed
- [ ] Note: local path is `/Users/springfield/dev/mcp-banana` but Go module path is `github.com/reshinto/mcp-banana` (matching the GitHub remote). This is intentional -- the local directory name does not need to match the module path.

---

## Task 0: Optimize .claude/ Configuration (Pre-Implementation)

**Model:** Sonnet
**Files:**
- Modify: `.claude/CLAUDE.md`
- Modify: `.claude/rules/architecture.md`
- Modify: `.claude/rules/coding-standards.md`
- Modify: `.claude/rules/testing.md`
- Modify: `.claude/rules/workflow.md`
- Modify: `.claude/rules/docker-ci-cd.md`
- Modify: `.claude/rules/docs.md`
- Modify: `.claude/settings.json` (Task 0 only: plugin normalization and hook registration)
- Modify: `.claude/settings.local.json` (deduplicate local overrides -- not committed)
- Modify: `.claude/hooks/security-patterns.txt`
- Modify: `.claude/hooks/session-end-unified-gate.sh`
- Modify: `.claude/hooks/plugin-profiles.json`
- Create: `.claude/hooks/enforce-naming-convention.sh`
- Create: `.claude/hooks/enforce-file-size.sh`
- Delete: `.claude/agents/ui-ux-designer.md` (no UI in this project)
- Delete: `.claude/agents/product-strategist.md` (not needed for infra)
- Delete: `.claude/skills/accessibility-audit/` (no UI)
- Delete: `.claude/skills/readme-optimization/` (covered by docs rule)
- Delete: `.claude/skills/strict-type-review/` (TypeScript-focused)
- Delete: `.claude/.scaffold-meta.json`

The current `.claude/` files were scaffolded from a generic multi-language template. They contain TypeScript/JavaScript/React/Python references, placeholder text, and rules irrelevant to this Go MCP server. Every token in these files is loaded into context on every Claude Code session, so irrelevant content wastes tokens and can mislead the AI.

**Checklist:**

- [ ] **Step 1: Rewrite `.claude/CLAUDE.md`**

Replace the placeholder content with mcp-banana-specific context:
- Architecture: Go MCP server with `mark3labs/mcp-go` + `google.golang.org/genai`, dual transport (stdio + HTTP), `internal/` packages for config, gemini, policy, security, server, tools
- Key paths: `cmd/mcp-banana/main.go`, `internal/gemini/registry.go` (model ID source of truth), `internal/security/` (security boundary), `.github/workflows/`
- Tech stack: Go 1.24, mark3labs/mcp-go, google.golang.org/genai, golang.org/x/time/rate, Docker distroless
- Security note: secrets must never appear in tool responses, logs, or errors. `genai.APIError` must be unwrapped safely.
- Remove: generic guidelines that duplicate rules files

- [ ] **Step 2: Rewrite `.claude/rules/architecture.md`**

Remove all non-Go content:
- Remove: state management (immer, spread operators), UI components, barrel exports, `index.ts`/`__init__.py`, `{{KEY_PATHS}}` placeholder, import alias references
- Replace with: Go MCP server architecture -- `cmd/` (entry point), `internal/config` (env loading), `internal/gemini` (API client + model registry), `internal/policy` (model recommendation), `internal/security` (validation + sanitization), `internal/server` (MCP server + middleware), `internal/tools` (MCP tool handlers)
- Add: security boundaries (Gemini client wraps all API access, security package gates all input/output)

- [ ] **Step 3: Rewrite `.claude/rules/coding-standards.md`**

Remove TypeScript/JS content, keep Go-only:
- Remove: `const`/`let`/`var`, `import type`, `unknown`, discriminated unions, `noUncheckedIndexedAccess`, type-only imports, barrel exports
- Keep: naming rules (no single-char variables, no non-standard abbreviations), DRY, Go formatting with `gofmt`
- Add: allowed standard Go abbreviations (`err`, `ctx`, `req`, `resp`, `cfg`, `srv`, `test` for `*testing.T`) -- no inline comments required, explained in README glossary
- Add: `test *testing.T` convention, context-specific error names (`loadError`, `validationError`, etc.), `.golangci.yml` disables `stylecheck`/`revive`
- Add: Go import grouping (stdlib, external, internal)
- Add: modularity rules -- max 500 lines per file, one responsibility per function, one objective per struct/type, DRY (extract shared logic into reusable functions), split files that grow beyond 500 lines into focused sub-files
- Add: centralize public-facing and cross-file strings as constants (error codes, safe messages, multi-file MIME types). Single-package inline literals are fine when meaning is clear from context.

- [ ] **Step 4: Rewrite `.claude/rules/testing.md`**

Remove non-Go conventions:
- Remove: `__tests__/` directories, `.test.<ext>`/`.spec.<ext>` naming, E2E dev server auto-start
- Replace with: Go conventions -- `_test.go` co-located with source, `go test -race -coverprofile`, `GeminiService` interface for mocking
- Add: security test requirements (verify no secrets in responses, verify `genai.APIError` unwrapping), CI command now includes `-race` and `-coverprofile`

- [ ] **Step 5: Simplify `.claude/rules/workflow.md`**

- Remove: 7-step development flow (Product Strategist -> UI/UX -> Tech Lead -> etc.) -- too heavyweight, wastes tokens
- Keep: branch strategy (mandatory), git operations, PR requirements, quality gate
- Simplify to: branch -> implement -> test -> review -> merge

- [ ] **Step 6: Rewrite `.claude/rules/docker-ci-cd.md`**

- Remove: generic deploy triggers table, generic CI pipeline description
- Replace with: actual mcp-banana CI/CD -- GitHub Actions CI on feature branches + PRs, CD on pushes to main, DigitalOcean deployment via SSH, auto-rollback on health check failure
- Add: distroless base image, `stop_grace_period: 120s`, `0.0.0.0` bind in container

- [ ] **Step 7: Simplify `.claude/rules/docs.md`**

- Remove: generic doc structure template with 6 sections
- Replace with: mcp-banana doc requirements -- README with architecture, threat model, Claude Code integration. Code comments on every exported function/type. Security-critical code needs SECURITY annotations.

- [ ] **Step 8: One-time normalize plugin declarations in `settings.json`, deduplicate `settings.local.json`**

This is a one-time pre-implementation cleanup. After Task 0, `settings.json` plugin entries are treated as fixed and always enabled. Future plugin preference changes happen only in `settings.local.json`.

**Step 8a: First, deduplicate `.claude/settings.local.json` (not committed):**
- Remove all 16 duplicate short-form entries (IDs like `"superpowers"`, `"context7"`, etc. that lack the `@` suffix)
- Keep only canonical IDs (`@claude-plugins-official` and `@dev-forge` format)
- Keep your local enabled/disabled overrides as desired
- Result: clean file with no duplicates. It may intentionally differ from `settings.json` because it stores personal local overrides.

**Step 8b: Then, normalize `.claude/settings.json` (committed -- one-time only):**
- Use the now-deduplicated `settings.local.json` canonical IDs as the source for any missing entries in `settings.json`
- Remove any duplicate or malformed plugin keys in `settings.json`
- Set ALL canonical plugin entries to `true` (settings.json is the complete canonical list with everything enabled)
- After this step, do not modify plugin settings in `settings.json` again

- [ ] **Step 9: Update `.claude/hooks/security-patterns.txt`**

Replace the current frontend-focused patterns (innerHTML, dangerouslySetInnerHTML, etc.) with Go/MCP-relevant security patterns:
```
\.Error\(\).*fmt\.Sprintf
\.Error\(\).*Errorf
exec\.Command
exec\.CommandContext
GEMINI_API_KEY
MCP_AUTH_TOKEN
AIza[0-9A-Za-z_-]{35}
0\.0\.0\.0:
WriteTimeout
```
These patterns flag: raw error forwarding, command injection, secret literals in code, public bind addresses, and the SSE-killing WriteTimeout. Note: `0\.0\.0\.0:` is intentionally flagged as a review signal -- the Docker container legitimately binds `0.0.0.0:8847`, so this pattern triggers review, not automatic rejection.

- [ ] **Step 10: Create `.claude/hooks/enforce-naming-convention.sh`**

Create an advisory hook that scans staged Go files for common naming violations. This is a **best-effort heuristic**, not a full Go parser -- it catches the most common forms of banned variables but cannot guarantee 100% coverage. The authoritative enforcement is the naming convention documented in `.claude/rules/coding-standards.md` and enforced during code review.

```bash
#!/usr/bin/env bash
# Advisory naming convention check on staged Go files.
# Catches common forms of banned abbreviated variables.
#
# LIMITATION: This uses regex, not a Go parser. It will catch most
# `:=` declarations but may miss some forms (e.g., function parameters)
# and may false-positive on string literals or comments. The authoritative
# rule is in .claude/rules/coding-standards.md -- this hook is a safety net.

STAGED_GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$')
if [ -z "$STAGED_GO_FILES" ]; then
  exit 0
fi

# Banned abbreviations in short-variable declarations (:=)
# NOTE: err, ctx, req, resp, cfg, srv are ALLOWED (standard Go idioms).
# This catches non-standard abbreviations only.
ABBREV_PATTERN='\b(mw|msg|sig|idx|num|val|buf|tmp|fn|cb|ch|opts|addr)\b\s*:='
VIOLATIONS=""

for FILE in $STAGED_GO_FILES; do
  # Skip test helper files and generated code
  if echo "$FILE" | grep -q '_generated\.go$'; then
    continue
  fi
  MATCHES=$(grep -nP "$ABBREV_PATTERN" "$FILE" 2>/dev/null || true)
  if [ -n "$MATCHES" ]; then
    VIOLATIONS="$VIOLATIONS\n$FILE:\n$MATCHES\n"
  fi
done

if [ -n "$VIOLATIONS" ]; then
  echo "NAMING ADVISORY: Possible abbreviated variable names in staged files:"
  echo -e "$VIOLATIONS"
  echo ""
  echo "Project convention requires full words (except allowed Go idioms: err, ctx, req, resp, cfg, srv, test)."
  echo "See .claude/rules/coding-standards.md for the complete naming policy."
  echo ""
  echo "If these are false positives (e.g., inside string literals), the commit can still proceed."
  # Hook exit codes in Claude Code:
  #   exit 0 = success (action proceeds)
  #   exit 1 = non-blocking error (surfaces result for review, action can continue)
  #   exit 2 = blocking error (action is prevented)
  # We use exit 1 because this is advisory, not authoritative enforcement.
  exit 1
fi
exit 0
```

- [ ] **Step 11: Create `.claude/hooks/enforce-file-size.sh`**

Advisory hook that flags Go files exceeding 500 lines. This enforces the modularity rule: files should be short, focused, and do one thing.

```bash
#!/usr/bin/env bash
# Advisory file-size check on staged Go files.
# Flags files exceeding 500 lines to encourage modular, focused files.
#
# The 500-line limit enforces: one responsibility per file, short functions,
# DRY principle, and easy-to-review code. Files that grow beyond this should
# be split into focused sub-files.

STAGED_GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$')
if [ -z "$STAGED_GO_FILES" ]; then
  exit 0
fi

MAX_LINES=500
VIOLATIONS=""

for FILE in $STAGED_GO_FILES; do
  LINE_COUNT=$(wc -l < "$FILE" | tr -d ' ')
  if [ "$LINE_COUNT" -gt "$MAX_LINES" ]; then
    VIOLATIONS="$VIOLATIONS\n  $FILE: $LINE_COUNT lines (max $MAX_LINES)"
  fi
done

if [ -n "$VIOLATIONS" ]; then
  echo "FILE SIZE ADVISORY: The following files exceed $MAX_LINES lines:"
  echo -e "$VIOLATIONS"
  echo ""
  echo "Split large files into focused sub-files with one responsibility each."
  echo "See .claude/rules/coding-standards.md for the modularity policy."
  # exit 0 = success, exit 1 = non-blocking advisory, exit 2 = blocking.
  # We use exit 1 because this is advisory, not authoritative enforcement.
  exit 1
fi
exit 0
```

- [ ] **Step 12: Register both hooks in `.claude/settings.json`**

Add a new matcher group to the `PreToolUse` array with both advisory hooks. Match the Bash tool at the group level, then filter to `git commit *` with `if`:
```json
{
  "matcher": "Bash",
  "hooks": [
    {
      "if": "Bash(git commit *)",
      "type": "command",
      "command": "bash ${CLAUDE_PROJECT_DIR}/.claude/hooks/enforce-naming-convention.sh",
      "timeout": 10000
    },
    {
      "if": "Bash(git commit *)",
      "type": "command",
      "command": "bash ${CLAUDE_PROJECT_DIR}/.claude/hooks/enforce-file-size.sh",
      "timeout": 10000
    }
  ]
}
```

This should be added as a new entry in the existing `PreToolUse` array alongside the existing matcher groups (block-ai-attribution, block-main-branch-commits, enforce-branch-naming). Do not overwrite or modify those existing entries.

- [ ] **Step 13: Update `.claude/hooks/session-end-unified-gate.sh`**

Update the quality gate script:
- Change `TEST_CMD` to include `-race` and `-coverprofile`:
  `TEST_CMD="go test -race -coverprofile=coverage.out ./..."`
- Remove E2E and Storybook references (not applicable)
- Keep the existing timeout and failure accumulation logic

- [ ] **Step 14: Update `.claude/hooks/plugin-profiles.json`**

This file is shared branch-mode metadata that controls which plugins auto-activate on specific branch patterns. It is safe to commit (it does not enable/disable plugins globally -- that is `settings.local.json`'s job). Remove frontend/UI branch mode entries that are irrelevant to this Go project:
- Remove: `feat/ui-`, `fix/ui-`, `feat/e2e-`, `fix/e2e-`, `feat/design-`, `feat/preview-`, `fix/preview-` entries (and their associated `frontend-design`, `figma`, `playground`, `playwright` plugins)
- Keep: `feat/backend-`, `fix/backend-`, `feat/api-`, `fix/api-`, `feat/refactor-`, `refactor/`, `chore/claude-`, `chore/skill-`, `chore/meta-` entries

- [ ] **Step 15: Delete irrelevant agents and skills**

```bash
rm .claude/agents/ui-ux-designer.md
rm .claude/agents/product-strategist.md
rm -rf .claude/skills/accessibility-audit
rm -rf .claude/skills/readme-optimization
rm -rf .claude/skills/strict-type-review
```

- [ ] **Step 16: Delete `.claude/.scaffold-meta.json`**

The scaffold metadata is no longer relevant since we've rewritten all scaffolded files:
```bash
rm .claude/.scaffold-meta.json
```

- [ ] **Step 17: Verify all `.claude/` files are internally consistent**

Quick check:
- Verify that `.claude/settings.json` contains every canonical plugin ID that was derived from `settings.local.json` in Step 8b, with all entries set to `true`
- `.claude/settings.local.json` has no duplicate short-form plugin entries -- not committed
- After Task 0, do not change plugin entries in `settings.json`. In this plan, `settings.json` is only modified within Task 0 for plugin normalization and hook registration.
- Plugin preference changes after Task 0 are made only in `.claude/settings.local.json`
- All hooks referenced in `settings.json` exist in `hooks/`
- No deleted agents/skills are referenced anywhere
- `CLAUDE.md` key paths match actual project structure

- [ ] **Step 18: Commit**

```bash
git add .claude/
git reset .claude/settings.local.json 2>/dev/null || true
# Confirm settings.local.json is not staged before committing:
if git diff --cached --name-only | grep -q '^\.claude/settings\.local\.json$'; then
  echo "ERROR: .claude/settings.local.json is staged"
  exit 1
fi
git commit -m "chore: optimize .claude/ config for Go MCP server, add advisory hooks"
```

---

## Task 1: Project Scaffolding

**Model:** Haiku
**Files:**
- Create: `go.mod`, `Makefile`, `.gitignore`, `.env.example`, `.dockerignore`, `.gitattributes`, `.golangci.yml`
- Create: `cmd/mcp-banana/main.go` (stub)
- Note: `internal/` subdirectories will be created naturally as files are added in later tasks

**Checklist:**

- [ ] **Step 1: Verify branch** (should already be on `feat/mcp-server` from Pre-Implementation Checklist)

```bash
git branch --show-current
```
Expected: `feat/mcp-server`

- [ ] **Step 2: Initialize Go module**

```bash
go mod init github.com/reshinto/mcp-banana
```

After init, verify `go.mod` contains `go 1.24` (or higher). If the installed Go version produces a different directive, explicitly edit `go.mod` to set `go 1.24` to match the project's minimum version requirement.

- [ ] **Step 3: Create .gitignore**

```gitignore
# Binaries
mcp-banana
*.exe

# Environment
.env

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store

# Test coverage
coverage.out
```

- [ ] **Step 4: Create .env.example**

```
GEMINI_API_KEY=
MCP_AUTH_TOKEN=
MCP_LOG_LEVEL=info
MCP_RATE_LIMIT=30
MCP_GLOBAL_CONCURRENCY=8
MCP_PRO_CONCURRENCY=3
MCP_MAX_IMAGE_BYTES=4194304
MCP_REQUEST_TIMEOUT_SECS=120
```

- [ ] **Step 5: Create .dockerignore**

```
.git
.claude
.env
*.md
LICENSE
docs/
```

- [ ] **Step 6: Create .gitattributes**

```
* text=auto eol=lf
*.go text eol=lf
Makefile text eol=lf
Dockerfile text eol=lf
*.sh text eol=lf
*.yml text eol=lf
*.yaml text eol=lf
```

- [ ] **Step 7: Create .golangci.yml**

The project naming convention overrides Go's standard naming rules. This config disables linters that would reject our naming choices like `test *testing.T`.

```yaml
version: "2"

linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - unused
    - gosimple
    - ineffassign
    - typecheck
  disable:
    - stylecheck
    - revive

  settings:
    govet:
      enable-all: true
```

- [ ] **Step 8: Create Makefile**

```makefile
.PHONY: build test lint fmt fmt-check vet run-stdio run-http clean rotate-token quality-gate

BINARY=mcp-banana
BUILD_FLAGS=-ldflags="-s -w" -trimpath

build:
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY) ./cmd/mcp-banana/

test:
	go test -coverprofile=coverage.out -race ./... -v

lint:
	golangci-lint run

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

vet:
	go vet ./...

run-stdio: build
	./$(BINARY) --transport stdio

run-http: build
	./$(BINARY) --transport http --addr 0.0.0.0:8847

clean:
	rm -f $(BINARY) coverage.out

rotate-token:
	@NEW_TOKEN=$$(openssl rand -hex 32); \
	echo "New token: $$NEW_TOKEN"; \
	echo ""; \
	echo "Step 1: Update MCP_AUTH_TOKEN on the server:"; \
	echo "  SSH into the server, edit /opt/mcp-banana/.env, set MCP_AUTH_TOKEN=$$NEW_TOKEN"; \
	echo "  Then restart: docker compose restart"; \
	echo ""; \
	echo "Step 2: Update your Claude Code config with the new token:"; \
	echo "  claude mcp add-json --scope user banana '{\"type\":\"http\",\"url\":\"http://localhost:8847/mcp\",\"headers\":{\"Authorization\":\"Bearer $$NEW_TOKEN\"}}'"

quality-gate: lint fmt-check vet test
	@echo "All checks passed"
```

- [ ] **Step 9: Create directory structure and stub main.go**

Create `cmd/mcp-banana/` directory (with `main.go` stub below). The `internal/` subdirectories will be created naturally when files are added in Tasks 2-10 -- Git does not track empty directories, so no `.gitkeep` files are needed.

Create `cmd/mcp-banana/main.go`:
```go
// Package main is the entry point for the mcp-banana server.
// It parses command-line flags and starts either the stdio or HTTP transport.
package main

import "fmt"

func main() {
	fmt.Println("mcp-banana starting")
}
```

- [ ] **Step 10: Verify build compiles**

```bash
go build ./cmd/mcp-banana/
```

- [ ] **Step 11: Commit**

```bash
git add .gitignore .gitattributes .golangci.yml .env.example .dockerignore Makefile go.mod cmd/
git commit -m "scaffold: initialize go module, linter config, and project structure"
```

---

## Task 2: Configuration Loading

**Model:** Haiku
**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Checklist:**

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:
```go
// Package config_test validates that the configuration loading correctly
// reads environment variables, applies defaults, and rejects invalid values.
package config_test

import (
	"testing"

	"github.com/reshinto/mcp-banana/internal/config"
)

// All tests use test.Setenv() exclusively for environment manipulation.
// test.Setenv automatically restores the original value after the test,
// eliminating cross-test pollution. To simulate a missing env var, use
// test.Setenv("VAR_NAME", "") -- empty string is invalid for required
// vars (like GEMINI_API_KEY) and triggers defaults for optional vars.

func TestLoad_ValidConfig(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")
	test.Setenv("MCP_AUTH_TOKEN", "abcdef0123456789abcdef0123456789ab")
	test.Setenv("MCP_LOG_LEVEL", "debug")

	serverConfig, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if serverConfig.GeminiAPIKey != "test-gemini-key-placeholder-for-unit-tests" {
		test.Errorf("unexpected API key: %s", serverConfig.GeminiAPIKey)
	}
	if serverConfig.LogLevel != "debug" {
		test.Errorf("unexpected log level: %s", serverConfig.LogLevel)
	}
}

func TestLoad_MissingAPIKey(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for missing API key")
	}
}

func TestLoad_DefaultLogLevel(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")
	test.Setenv("MCP_LOG_LEVEL", "")

	serverConfig, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if serverConfig.LogLevel != "info" {
		test.Errorf("expected default log level 'info', got: %s", serverConfig.LogLevel)
	}
}

func TestLoad_InvalidLogLevel(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")
	test.Setenv("MCP_LOG_LEVEL", "GARBAGE")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for invalid log level")
	}
}

func TestLoad_MalformedIntegerEnvVar(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")
	test.Setenv("MCP_RATE_LIMIT", "abc")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for malformed integer env var")
	}
}

func TestLoad_ZeroRateLimit(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")
	test.Setenv("MCP_RATE_LIMIT", "0")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for zero rate limit")
	}
}

func TestLoad_NegativeConcurrency(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")
	test.Setenv("MCP_PRO_CONCURRENCY", "-1")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for negative concurrency")
	}
}

func TestLoad_ProConcurrencyExceedsGlobal(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")
	test.Setenv("MCP_GLOBAL_CONCURRENCY", "4")
	test.Setenv("MCP_PRO_CONCURRENCY", "10")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error when pro concurrency exceeds global concurrency")
	}
}

func TestLoad_ZeroTimeout(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")
	test.Setenv("MCP_REQUEST_TIMEOUT_SECS", "0")

	_, loadError := config.Load()
	if loadError == nil {
		test.Fatal("expected error for zero timeout")
	}
}

func TestLoad_DefaultLimits(test *testing.T) {
	test.Setenv("GEMINI_API_KEY", "test-gemini-key-placeholder-for-unit-tests")

	serverConfig, loadError := config.Load()
	if loadError != nil {
		test.Fatalf("unexpected error: %v", loadError)
	}
	if serverConfig.RateLimit != 30 {
		test.Errorf("expected default rate limit 30, got %d", serverConfig.RateLimit)
	}
	if serverConfig.GlobalConcurrency != 8 {
		test.Errorf("expected default global concurrency 8, got %d", serverConfig.GlobalConcurrency)
	}
	if serverConfig.RequestTimeoutSecs != 120 {
		test.Errorf("expected default timeout 120, got %d", serverConfig.RequestTimeoutSecs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/... -v
```

- [ ] **Step 3: Write implementation**

Create `internal/config/config.go`:
```go
// Package config handles loading and validating server configuration from
// environment variables. It provides a single Config struct that holds all
// runtime settings. Secrets (API keys, tokens) are stored here at startup
// and must NEVER be exposed in tool responses, logs, or error messages.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all server configuration loaded from environment variables.
// This struct is created once at startup and passed to components that need it.
//
// SECURITY: GeminiAPIKey and AuthToken are secrets. They must never appear
// in any tool response, log output, error message, or health check.
type Config struct {
	GeminiAPIKey       string // The Google Gemini API key for authentication
	AuthToken          string // Bearer token for HTTP transport auth (optional for stdio)
	LogLevel           string // Logging verbosity: "debug", "info", "warn", "error"
	RateLimit          int    // Maximum requests per minute (default: 30)
	GlobalConcurrency  int    // Maximum simultaneous requests across all models (default: 8)
	ProConcurrency     int    // Maximum simultaneous requests for Pro model (default: 3)
	MaxImageBytes      int    // Maximum decoded image size in bytes (default: 4MB)
	RequestTimeoutSecs int    // Timeout for each Gemini API call in seconds (default: 120)
}

// Load reads configuration from environment variables and validates it.
// Returns an error immediately if required values are missing or malformed.
// This "fail fast" approach ensures misconfiguration is caught at startup,
// not when the first request arrives.
func Load() (*Config, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("GEMINI_API_KEY is required")
	}

	authToken := os.Getenv("MCP_AUTH_TOKEN")

	logLevel := strings.ToLower(os.Getenv("MCP_LOG_LEVEL"))
	if logLevel == "" {
		logLevel = "info"
	}
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[logLevel] {
		return nil, errors.New("MCP_LOG_LEVEL must be one of: debug, info, warn, error")
	}

	rateLimit, rateLimitError := getEnvInt("MCP_RATE_LIMIT", 30)
	if rateLimitError != nil {
		return nil, rateLimitError
	}
	globalConcurrency, concurrencyError := getEnvInt("MCP_GLOBAL_CONCURRENCY", 8)
	if concurrencyError != nil {
		return nil, concurrencyError
	}
	proConcurrency, proError := getEnvInt("MCP_PRO_CONCURRENCY", 3)
	if proError != nil {
		return nil, proError
	}
	maxImageBytes, imageBytesError := getEnvInt("MCP_MAX_IMAGE_BYTES", 4*1024*1024)
	if imageBytesError != nil {
		return nil, imageBytesError
	}
	requestTimeoutSecs, timeoutError := getEnvInt("MCP_REQUEST_TIMEOUT_SECS", 120)
	if timeoutError != nil {
		return nil, timeoutError
	}

	// Validate that all numeric limits are positive (zero or negative values
	// would cause broken rate limiting, deadlocked semaphores, or disabled timeouts).
	if rateLimit <= 0 {
		return nil, errors.New("MCP_RATE_LIMIT must be a positive integer")
	}
	if globalConcurrency <= 0 {
		return nil, errors.New("MCP_GLOBAL_CONCURRENCY must be a positive integer")
	}
	if proConcurrency <= 0 {
		return nil, errors.New("MCP_PRO_CONCURRENCY must be a positive integer")
	}
	if proConcurrency > globalConcurrency {
		return nil, fmt.Errorf("MCP_PRO_CONCURRENCY (%d) must be <= MCP_GLOBAL_CONCURRENCY (%d)", proConcurrency, globalConcurrency)
	}
	if maxImageBytes <= 0 {
		return nil, errors.New("MCP_MAX_IMAGE_BYTES must be a positive integer")
	}
	if requestTimeoutSecs <= 0 {
		return nil, errors.New("MCP_REQUEST_TIMEOUT_SECS must be a positive integer")
	}

	return &Config{
		GeminiAPIKey:       apiKey,
		AuthToken:          authToken,
		LogLevel:           logLevel,
		RateLimit:          rateLimit,
		GlobalConcurrency:  globalConcurrency,
		ProConcurrency:     proConcurrency,
		MaxImageBytes:      maxImageBytes,
		RequestTimeoutSecs: requestTimeoutSecs,
	}, nil
}

// getEnvInt reads an integer from an environment variable, returning the
// provided default if the variable is unset. Returns an error if the
// variable is set but cannot be parsed as an integer (fail fast on typos).
func getEnvInt(name string, defaultValue int) (int, error) {
	rawValue := os.Getenv(name)
	if rawValue == "" {
		return defaultValue, nil
	}
	parsed, parseError := strconv.Atoi(rawValue)
	if parseError != nil {
		return 0, fmt.Errorf("%s must be a valid integer, got %q", name, rawValue)
	}
	return parsed, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/config/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add configuration loading with configurable limits"
```

---

## Task 3: Model Registry

**Model:** Haiku
**Files:**
- Create: `internal/gemini/registry.go`
- Create: `internal/gemini/registry_test.go`

**Checklist:**

- [ ] **Step 1: Write the failing test**

Create `internal/gemini/registry_test.go`:
```go
// Package gemini_test validates the model registry that maps Nano Banana
// aliases to internal Gemini model identifiers.
package gemini_test

import (
	"testing"

	"github.com/reshinto/mcp-banana/internal/gemini"
)

func TestLookupModel_ValidAlias(test *testing.T) {
	model, lookupError := gemini.LookupModel("nano-banana-2")
	if lookupError != nil {
		test.Fatalf("unexpected error: %v", lookupError)
	}
	if model.GeminiID == "" {
		test.Error("expected non-empty GeminiID")
	}
	if model.Alias != "nano-banana-2" {
		test.Errorf("expected alias 'nano-banana-2', got '%s'", model.Alias)
	}
}

func TestLookupModel_AllAliases(test *testing.T) {
	aliases := []string{"nano-banana-2", "nano-banana-pro", "nano-banana-original"}
	for _, alias := range aliases {
		model, lookupError := gemini.LookupModel(alias)
		if lookupError != nil {
			test.Errorf("alias %q: unexpected error: %v", alias, lookupError)
			continue
		}
		if model.GeminiID == "" {
			test.Errorf("alias %q: empty GeminiID", alias)
		}
	}
}

func TestLookupModel_InvalidAlias(test *testing.T) {
	_, lookupError := gemini.LookupModel("not-a-real-model")
	if lookupError == nil {
		test.Fatal("expected error for invalid alias")
	}
}

func TestAllModelsSafe_ReturnsThreeModels(test *testing.T) {
	models := gemini.AllModelsSafe()
	if len(models) != 3 {
		test.Fatalf("expected 3 models, got %d", len(models))
	}
}

func TestAllModelsSafe_IsSortedByAlias(test *testing.T) {
	models := gemini.AllModelsSafe()
	for position := 1; position < len(models); position++ {
		if models[position-1].Alias > models[position].Alias {
			test.Errorf("models not sorted: %s > %s", models[position-1].Alias, models[position].Alias)
		}
	}
}

func TestAllModelsSafe_NeverExposesGeminiID(test *testing.T) {
	// SECURITY: SafeModelInfo must NOT contain a GeminiID field.
	// This test verifies that the safe type does not expose internal model IDs.
	models := gemini.AllModelsSafe()
	for _, model := range models {
		if model.Alias == "" {
			test.Error("expected non-empty alias in safe model info")
		}
		// SafeModelInfo struct has no GeminiID field -- this is enforced by the type system.
		// If someone adds a GeminiID field to SafeModelInfo, this test file will need updating.
	}
}

func TestValidateRegistryAtStartup_RejectsSentinelIDs(test *testing.T) {
	// This test verifies that ValidateRegistryAtStartup correctly detects
	// sentinel IDs and rejects them. It passes in both development (sentinels
	// present -> error returned) and after release (real IDs -> no error).
	//
	// The test is self-enforcing: it checks whether sentinels exist and
	// asserts the validation result matches. No manual flip needed.
	startupError := gemini.ValidateRegistryAtStartup()
	hasSentinels := false
	for _, model := range gemini.AllModelsSafe() {
		// Check via LookupModel which returns internal ModelInfo with GeminiID
		if fullModel, lookupError := gemini.LookupModel(model.Alias); lookupError == nil {
			if fullModel.GeminiID == "VERIFY_MODEL_ID_BEFORE_RELEASE" {
				hasSentinels = true
				break
			}
		}
	}

	if hasSentinels && startupError == nil {
		test.Fatal("sentinel IDs are present but ValidateRegistryAtStartup did not return an error")
	}
	if !hasSentinels && startupError != nil {
		test.Fatalf("no sentinel IDs remain but ValidateRegistryAtStartup returned error: %v", startupError)
	}
}

func TestValidAliases_ReturnsThreeStrings(test *testing.T) {
	aliases := gemini.ValidAliases()
	if len(aliases) != 3 {
		test.Fatalf("expected 3 aliases, got %d", len(aliases))
	}
}

func TestValidAliases_IsSorted(test *testing.T) {
	aliases := gemini.ValidAliases()
	for position := 1; position < len(aliases); position++ {
		if aliases[position-1] > aliases[position] {
			test.Errorf("aliases not sorted: %s > %s", aliases[position-1], aliases[position])
		}
	}
}
```

- [ ] **Step 2: Run tests before implementation to confirm missing code paths are surfaced**

```bash
go test ./internal/gemini/... -v
```

- [ ] **Step 3: Write implementation**

Create `internal/gemini/registry.go`:
```go
// Package gemini provides the Gemini API client and model registry for
// mcp-banana. It maps Nano Banana model aliases to internal Gemini model
// identifiers and wraps all Gemini API interactions.
//
// SECURITY: Gemini model IDs (GeminiID) are internal-only and must NEVER
// appear in any MCP tool response, log entry, or error message returned
// to Claude Code. Only the Nano Banana aliases are safe to expose.
package gemini

import (
	"fmt"
	"sort"
)

// ModelInfo describes a Nano Banana model and its mapping to a Gemini model ID.
// This type is INTERNAL -- use SafeModelInfo for data returned to Claude Code.
//
// SECURITY: The GeminiID field must never be included in any response to
// Claude Code. It is used only for internal Gemini API calls.
type ModelInfo struct {
	Alias          string
	GeminiID       string // INTERNAL ONLY -- never expose to Claude Code
	Description    string
	Capabilities   []string
	TypicalLatency string
	BestFor        string
}

// SafeModelInfo contains only the fields safe to expose to Claude Code.
// It deliberately excludes GeminiID to prevent accidental leakage of
// internal Gemini model identifiers.
type SafeModelInfo struct {
	Alias          string   `json:"id"`
	Description    string   `json:"description"`
	Capabilities   []string `json:"capabilities"`
	TypicalLatency string   `json:"typical_latency"`
	BestFor        string   `json:"best_for"`
}

// registry is the single source of truth for model alias-to-ID mapping.
//
// IMPORTANT: The GeminiID values MUST be verified against the live Gemini
// API model catalog before release. The sentinel value
// `VERIFY_MODEL_ID_BEFORE_RELEASE` will cause a startup failure (see
// ValidateRegistryAtStartup) to prevent accidental deployment with
// unverified IDs.
//
// To verify: call the Gemini models.list API or check
// https://ai.google.dev/gemini-api/docs/models
//
// Expected mappings to verify before release:
//   nano-banana-2   -> gemini-3.1-flash-image-preview
//   nano-banana-pro -> gemini-3-pro-image-preview
//
// nano-banana-original is a PROJECT-INTERNAL alias (not an official
// Google model name) for the speed-optimized Nano Banana model, which
// Google documents as backed by gemini-2.5-flash-image. This alias is
// PROVISIONAL -- if no confirmed Gemini model ID can be verified for it,
// remove it from the registry before release.
var registry = map[string]ModelInfo{
	"nano-banana-2": {
		Alias:          "nano-banana-2",
		GeminiID:       "VERIFY_MODEL_ID_BEFORE_RELEASE",
		Description:    "Fast, high-volume image generation. Under 10 seconds.",
		Capabilities:   []string{"generate", "edit"},
		TypicalLatency: "5-10s",
		BestFor:        "Iterative work, drafts, batch generation",
	},
	"nano-banana-pro": {
		Alias:          "nano-banana-pro",
		GeminiID:       "VERIFY_MODEL_ID_BEFORE_RELEASE",
		Description:    "Professional quality with advanced reasoning. 15-45 seconds.",
		Capabilities:   []string{"generate", "edit"},
		TypicalLatency: "15-45s",
		BestFor:        "Final assets, photorealistic images, complex scenes",
	},
	"nano-banana-original": {
		Alias:          "nano-banana-original",
		GeminiID:       "VERIFY_MODEL_ID_BEFORE_RELEASE",
		Description:    "Speed and efficiency optimized. 3-8 seconds.",
		Capabilities:   []string{"generate", "edit"},
		TypicalLatency: "3-8s",
		BestFor:        "Quick previews, high-volume batch work",
	},
}

// LookupModel returns the full ModelInfo (including GeminiID) for internal use.
// Returns an error if the alias is not in the allowlist.
// SECURITY: The returned ModelInfo contains GeminiID -- do not expose to clients.
func LookupModel(alias string) (ModelInfo, error) {
	model, exists := registry[alias]
	if !exists {
		return ModelInfo{}, fmt.Errorf("unknown model alias: %q", alias)
	}
	return model, nil
}

// AllModelsSafe returns all registered models as SafeModelInfo (no GeminiID).
// The output is sorted by alias for deterministic ordering.
// Use this for any data that will be returned to Claude Code.
func AllModelsSafe() []SafeModelInfo {
	models := make([]SafeModelInfo, 0, len(registry))
	for _, model := range registry {
		models = append(models, SafeModelInfo{
			Alias:          model.Alias,
			Description:    model.Description,
			Capabilities:   model.Capabilities,
			TypicalLatency: model.TypicalLatency,
			BestFor:        model.BestFor,
		})
	}
	sort.Slice(models, func(first, second int) bool {
		return models[first].Alias < models[second].Alias
	})
	return models
}

// ValidAliases returns the list of valid model alias strings.
func ValidAliases() []string {
	aliases := make([]string, 0, len(registry))
	for alias := range registry {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

// ValidateRegistryAtStartup checks that no model alias still has a sentinel
// GeminiID. This prevents accidental deployment with unverified model IDs.
// Call this during server startup before accepting any requests.
func ValidateRegistryAtStartup() error {
	for alias, model := range registry {
		if model.GeminiID == "VERIFY_MODEL_ID_BEFORE_RELEASE" {
			return fmt.Errorf("model %q has unverified GeminiID -- verify at https://ai.google.dev/gemini-api/docs/models before release", alias)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/gemini/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/gemini/registry.go internal/gemini/registry_test.go
git commit -m "feat: add model registry with safe DTO and sorted output"
```

---

## Task 4: Gemini Error Handling

**Model:** Sonnet
**Files:**
- Create: `internal/gemini/errors.go`
- Create: `internal/gemini/errors_test.go`

This is a **security-critical** file. The genai SDK's `APIError` type can contain request metadata. We must NEVER forward raw error text.

**Checklist:**

- [ ] **Step 1: Fetch the genai dependency and verify APIError shape**

```bash
go get google.golang.org/genai
go doc google.golang.org/genai.APIError
```

Confirm `genai.APIError` has `Code int` and `Message string` fields. Verify that both pointer-form (`&genai.APIError{Code: 400}`) and value-form construction compile, and that `errors.As` works against both `*genai.APIError` and `genai.APIError`. If the struct shape has changed, adapt both the test and implementation code below before writing any files.

- [ ] **Step 2: Write the failing test**

Create `internal/gemini/errors_test.go`:
```go
// Package gemini_test validates the safe error mapping that prevents
// Gemini SDK error details from leaking to Claude Code.
package gemini_test

import (
	"errors"
	"testing"

	"github.com/reshinto/mcp-banana/internal/gemini"
	"google.golang.org/genai"
)

func TestMapError_NilError(test *testing.T) {
	code, message := gemini.MapError(nil)
	if code != "" || message != "" {
		test.Errorf("expected empty code/message for nil error, got %q/%q", code, message)
	}
}

func TestMapError_GenericError(test *testing.T) {
	code, message := gemini.MapError(errors.New("something broke"))
	if code != "generation_failed" {
		test.Errorf("expected 'generation_failed', got %q", code)
	}
	if message == "" {
		test.Error("expected non-empty message")
	}
}

func TestMapError_NeverLeaksRawText(test *testing.T) {
	// SECURITY: Verify that raw error text (which could contain API keys
	// or request headers) is never included in the safe output.
	sensitiveError := errors.New("Authorization: Bearer " + "AIza" + "SySecretKeyHere12345678901234567")
	code, message := gemini.MapError(sensitiveError)
	if code == "" {
		test.Error("expected non-empty code")
	}
	if containsSubstring(message, "AIzaSy") {
		test.Fatal("SECURITY: API key pattern leaked into error message")
	}
	if containsSubstring(message, "Authorization") {
		test.Fatal("SECURITY: auth header leaked into error message")
	}
}

func TestMapError_BadRequest(test *testing.T) {
	apiError := &genai.APIError{Code: 400, Message: "blocked by safety"}
	code, _ := gemini.MapError(apiError)
	if code != "content_policy_violation" {
		test.Errorf("expected content_policy_violation for 400, got %q", code)
	}
}

func TestMapError_Forbidden(test *testing.T) {
	apiError := &genai.APIError{Code: 403, Message: "forbidden"}
	code, _ := gemini.MapError(apiError)
	if code != "content_policy_violation" {
		test.Errorf("expected content_policy_violation for 403, got %q", code)
	}
}

func TestMapError_NotFound(test *testing.T) {
	apiError := &genai.APIError{Code: 404, Message: "model not found"}
	code, _ := gemini.MapError(apiError)
	if code != "model_unavailable" {
		test.Errorf("expected model_unavailable for 404, got %q", code)
	}
}

func TestMapError_TooManyRequests(test *testing.T) {
	apiError := &genai.APIError{Code: 429, Message: "quota exceeded"}
	code, _ := gemini.MapError(apiError)
	if code != "quota_exceeded" {
		test.Errorf("expected quota_exceeded for 429, got %q", code)
	}
}

func TestMapError_ServerError(test *testing.T) {
	apiError := &genai.APIError{Code: 500, Message: "internal error with key=" + "AIza" + "SySecret"}
	code, message := gemini.MapError(apiError)
	if code != "generation_failed" {
		test.Errorf("expected generation_failed for 500, got %q", code)
	}
	// SECURITY: verify the raw Message with the key pattern is NOT returned
	if containsSubstring(message, "AIzaSy") {
		test.Fatal("SECURITY: API key pattern leaked from 500 error")
	}
}

func containsSubstring(haystack, needle string) bool {
	return len(haystack) >= len(needle) && searchSubstring(haystack, needle)
}

func searchSubstring(haystack, needle string) bool {
	for position := 0; position <= len(haystack)-len(needle); position++ {
		if haystack[position:position+len(needle)] == needle {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/gemini/... -v -run TestMapError
```

- [ ] **Step 4: Write implementation**

Create `internal/gemini/errors.go`:
```go
// Package gemini provides the Gemini API client and model registry.
//
// SECURITY: This file contains the safe error mapping boundary. Raw SDK error
// text (which can contain API keys and request headers) is never forwarded
// to Claude Code. All errors are mapped to predefined safe codes and messages.
package gemini

import (
	"errors"
	"net/http"
	"strings"

	"google.golang.org/genai"
)

// Error codes returned to Claude Code. These are the ONLY error strings
// that should ever reach the client. Never forward raw SDK error text.
//
// SECURITY: These constants form a strict allowlist. No other error text
// may be returned to callers. This prevents API keys, internal URLs,
// and SDK diagnostics from leaking through error responses.
const (
	ErrContentPolicy  = "content_policy_violation"
	ErrQuotaExceeded  = "quota_exceeded"
	ErrModelUnavail   = "model_unavailable"
	ErrGenerationFail = "generation_failed"
	ErrServerError    = "server_error"
)

// safeMessages maps error codes to safe, human-readable messages.
// These messages contain NO internal details, no SDK error text, no headers.
var safeMessages = map[string]string{
	ErrContentPolicy:  "The prompt was blocked by content safety policy. Rephrase and try again.",
	ErrQuotaExceeded:  "API quota exceeded. Try again later.",
	ErrModelUnavail:   "The requested model is currently unavailable.",
	ErrGenerationFail: "Image generation failed. This may be transient -- retry is safe.",
	ErrServerError:    "An internal server error occurred.",
}

// MapError takes a raw error from the Gemini genai SDK and returns a safe
// (code, message) pair. The raw error text is NEVER included in the output.
//
// SECURITY: This is a critical security boundary. The genai SDK's APIError
// type can contain request metadata. We extract ONLY the HTTP status code
// and map it to a predefined safe message. The raw Message field is discarded.
//
// NOTE: This uses genai.APIError (from google.golang.org/genai), NOT the
// legacy googleapi.Error (from google.golang.org/api).
func MapError(inputError error) (code string, message string) {
	if inputError == nil {
		return "", ""
	}

	// Try to unwrap as a genai API error for HTTP status-based classification.
	// Handle pointer form (typical SDK wrapping) and value form (if SDK ever
	// returns unwrapped value errors). Both forms are tested.
	var apiErrorPointer *genai.APIError
	if errors.As(inputError, &apiErrorPointer) {
		safeCode := mapHTTPStatus(apiErrorPointer.Code)
		return safeCode, safeMessages[safeCode]
	}
	var apiErrorValue genai.APIError
	if errors.As(inputError, &apiErrorValue) {
		safeCode := mapHTTPStatus(apiErrorValue.Code)
		return safeCode, safeMessages[safeCode]
	}

	// Fallback: classify by substring patterns in the error message.
	// SECURITY: We read the error text for classification only. We never
	// include any part of this text in the returned message.
	lowercaseError := strings.ToLower(inputError.Error())
	switch {
	case strings.Contains(lowercaseError, "safety") || strings.Contains(lowercaseError, "blocked"):
		return ErrContentPolicy, safeMessages[ErrContentPolicy]
	case strings.Contains(lowercaseError, "quota") || strings.Contains(lowercaseError, "rate"):
		return ErrQuotaExceeded, safeMessages[ErrQuotaExceeded]
	case strings.Contains(lowercaseError, "not found") || strings.Contains(lowercaseError, "deprecated"):
		return ErrModelUnavail, safeMessages[ErrModelUnavail]
	default:
		return ErrGenerationFail, safeMessages[ErrGenerationFail]
	}
}

// mapHTTPStatus converts an HTTP status code into a safe error code string.
func mapHTTPStatus(status int) string {
	switch {
	case status == http.StatusBadRequest:
		return ErrContentPolicy
	case status == http.StatusForbidden:
		return ErrContentPolicy
	case status == http.StatusNotFound:
		return ErrModelUnavail
	case status == http.StatusTooManyRequests:
		return ErrQuotaExceeded
	case status >= 500:
		return ErrGenerationFail
	default:
		return ErrGenerationFail
	}
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/gemini/... -v -run TestMapError
```

- [ ] **Step 6: Commit**

```bash
git add internal/gemini/errors.go internal/gemini/errors_test.go go.mod go.sum
git commit -m "feat: add safe Gemini error mapping using genai.APIError"
```

---

## Task 5: Input Validation

**Model:** Sonnet
**Files:**
- Create: `internal/security/validate.go`
- Create: `internal/security/validate_test.go`

**Checklist:**

- [ ] **Step 1: Write the failing test**

Create `internal/security/validate_test.go` with tests covering: valid prompts, empty prompts, too-long prompts (10001 runes), boundary (exactly 10000 runes accepted), null bytes, all valid model aliases, invalid aliases, empty optional alias, all aspect ratios, invalid ratio, all priorities, invalid priority, all MIME types, invalid MIME, oversized base64, valid base64, invalid base64, empty base64, magic byte mismatch, task description tests.

Full test file should have ~20 test functions covering all validators with boundary and negative cases.

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Write implementation**

Create `internal/security/validate.go`:

Key implementation details:
- Use `utf8.RuneCountInString(prompt)` for length checks (runes, not bytes)
- `ValidateAndDecodeImage(encoded string, mimeType string, maxDecodedBytes int) ([]byte, error)` -- validates AND returns decoded bytes (prevents double-decode in edit handler). The `maxDecodedBytes` parameter comes from `Config.MaxImageBytes`.
- After base64 decode, check `len(decoded) < 12` before magic byte checks
- Validate magic bytes: PNG `\x89PNG`, JPEG `\xFF\xD8\xFF`, WebP `RIFF`+`WEBP` at offset 8

- [ ] **Step 4: Run tests**

- [ ] **Step 5: Commit**

```bash
git add internal/security/
git commit -m "feat: add input validation with rune-length checks and magic-byte image validation"
```

---

## Task 6: Output Sanitization

**Model:** Sonnet
**Files:**
- Create: `internal/security/sanitize.go`
- Create: `internal/security/sanitize_test.go`

**Checklist:**

- [ ] **Step 1: Write tests**

Include tests for: no secrets (unchanged), redacts API key pattern, redacts registered secret, multiple secrets in one string, empty string input, newline injection, carriage return. Every test uses `test.Cleanup(security.ClearSecrets)`.

- [ ] **Step 2: Write implementation**

Key: include `ClearSecrets()` function for test isolation:
```go
// ClearSecrets removes all registered secrets. Used only in tests
// to prevent state leakage between test functions.
func ClearSecrets() {
	secretsMutex.Lock()
	defer secretsMutex.Unlock()
	registeredSecrets = nil
}
```

- [ ] **Step 3: Run tests, commit**

---

## Task 7: Model Recommendation Policy

**Model:** Sonnet
**Files:**
- Create: `internal/policy/selector.go`
- Create: `internal/policy/selector_test.go`

**Specification (self-contained):**

`Recommend(taskDescription string, priority string) Recommendation` is a pure function with no side effects.

Input:
- `taskDescription`: free-text description of the image task
- `priority`: one of `"speed"`, `"quality"`, `"balanced"`, or `""` (treated as `"balanced"`). Any unrecognized priority value (e.g., `"foo"`, `"fast"`) is treated as `"balanced"` -- no error, silent normalization.

Decision rules:
- `"speed"` -> always return `nano-banana-original`
- `"quality"` -> always return `nano-banana-pro`
- `"balanced"` (or empty, or any unrecognized value):
  - If `taskDescription` contains any of: "professional", "photorealistic", "detailed", "complex", "final" -> return `nano-banana-pro`
  - If `taskDescription` contains any of: "quick", "draft", "sketch", "iterate", "batch", "preview" -> return `nano-banana-original`
  - Otherwise (default) -> return `nano-banana-2`
- Keyword matching is case-insensitive (`strings.ToLower` before matching)
- First matching keyword wins (pro keywords checked before speed keywords)

Output struct:
```go
type Recommendation struct {
    RecommendedModel string        `json:"recommended_model"`
    Reason           string        `json:"reason"`
    Alternatives     []Alternative `json:"alternatives"`
}
type Alternative struct {
    Model    string `json:"model"`
    Tradeoff string `json:"tradeoff"`
}
```

Each recommendation must include at least one alternative with a tradeoff explanation. Alternatives are ordered by relevance: the most likely useful alternative first. The recommended model never appears in the alternatives list.

Alternative ordering by recommendation:
- If recommended is `nano-banana-pro`: alternatives are `[nano-banana-2 (faster, lower cost), nano-banana-original (fastest, basic quality)]`
- If recommended is `nano-banana-original`: alternatives are `[nano-banana-2 (better quality, moderate speed), nano-banana-pro (best quality, slowest)]`
- If recommended is `nano-banana-2`: alternatives are `[nano-banana-pro (higher quality, slower), nano-banana-original (faster, lower quality)]`

Reason strings describe why the model was chosen (e.g., `"Speed priority: fastest model selected"`, `"Balanced: keyword 'professional' suggests high-quality output"`, `"Balanced: default mid-tier model for general tasks"`). **Tests must NOT assert exact reason string wording** -- instead test that the reason is non-empty and contains the expected priority or keyword context. This allows reason text to be refined without breaking tests.

Tests must cover: speed priority, quality priority, balanced with pro keywords, balanced with speed keywords, balanced default, empty priority, unrecognized priority (treated as balanced), case-insensitive matching, recommended model not in alternatives, reason is non-empty for all paths.

---

## Task 8: Gemini API Client Wrapper

**Model:** Sonnet
**Files:**
- Create: `internal/gemini/client.go`
- Create: `internal/gemini/service.go` (interface definition)

> **File-size note:** `internal/gemini/` is a high-pressure package (client, registry, service interface, request builders, image extraction). If `client.go` approaches 500 lines, split request-building helpers into `internal/gemini/request.go` and image extraction into `internal/gemini/extract.go`.
- Create: `internal/gemini/client_test.go`

**Checklist:**

- [ ] **Step 1: Create the GeminiService interface**

Create `internal/gemini/service.go`:
```go
// Package gemini provides the Gemini API client and model registry.
//
// GeminiService defines the interface for image generation operations,
// enabling dependency injection and mock testing without a real API key.
package gemini

import "context"

// GeminiService defines the interface for image generation operations.
// This interface enables dependency injection and mock testing without
// requiring a real Gemini API key or network access.
//
// The *Client type satisfies this interface. Tests can use a mock
// implementation to verify handler behavior in isolation.
type GeminiService interface {
	// GenerateImage creates a new image from a text prompt.
	// modelAlias is a Nano Banana alias (not a Gemini model ID).
	GenerateImage(requestContext context.Context, modelAlias string, prompt string, options GenerateOptions) (*ImageResult, error)

	// EditImage modifies an existing image using text instructions.
	// imageData is the raw decoded bytes (not base64).
	EditImage(requestContext context.Context, modelAlias string, imageData []byte, mimeType string, instructions string) (*ImageResult, error)
}

// GenerateOptions holds optional parameters for image generation.
type GenerateOptions struct {
	AspectRatio string
}

// ImageResult is the safe output of image generation/editing.
// Contains only data safe to return to Claude Code.
type ImageResult struct {
	ImageBase64    string `json:"image_base64"`
	MIMEType       string `json:"mime_type"`
	ModelUsed      string `json:"model_used"`        // Nano Banana alias, NOT Gemini ID
	GenerationTime int64  `json:"generation_time_ms"`
}
```

- [ ] **Step 2: Create the client implementation**

Create `internal/gemini/client.go`:

**Testing strategy:** The `Client` struct wraps `genai.Client` which requires network access. To enable unit testing without live Gemini calls:
- `extractImage` must be a standalone function (not a method) that takes `*genai.GenerateContentResponse` and returns `*ImageResult, error`. Test it directly with constructed response objects.
- Pro semaphore behavior is testable by creating a `Client` with a small semaphore and verifying slot acquisition/release with context cancellation.
- **Request-building seam:** Extract request construction into pure helper functions that return the **full set of SDK inputs** needed by `Models.GenerateContent`, not just the config. Specifically:
  - `buildGenerateInputs(prompt string, modelID string, aspectRatio string) (modelName string, contents []*genai.Content, config *genai.GenerateContentConfig)` -- returns the model name, content parts (prompt as a text Part), and config (aspect ratio, safety settings).
  - `buildEditInputs(prompt string, modelID string, imageData []byte, mimeType string) (modelName string, contents []*genai.Content, config *genai.GenerateContentConfig)` -- returns model name, content parts (prompt text Part + image inline data Part with correct MIME type), and config.
  - These are standalone functions (not methods). Test them directly to verify: prompt-to-parts mapping, image bytes/MIME landing in the correct `genai.Part`, model ID propagation into the model name string, aspect ratio in config, and safety settings. This covers the full request mapping, not just config.
- Full `GenerateImage`/`EditImage` methods (which call `client.inner.Models.GenerateContent` with the outputs of these helpers) are tested via the `GeminiService` interface mock in Task 9's handler tests, not here. The seam above ensures the request-building logic is independently verified.

Key implementation details:
- `NewClient(startupContext, apiKey, timeoutSecs, proConcurrency)` takes a startup-scoped context (used only for SDK client initialization, NOT request-scoped). Stores timeout and semaphore config.
- Every `GenerateContent` call is wrapped with `context.WithTimeout`:
```go
callContext, cancelCall := context.WithTimeout(requestContext, time.Duration(client.timeoutSecs)*time.Second)
defer cancelCall()
result, generateError := client.inner.Models.GenerateContent(callContext, ...)
```
- `extractImage` handles: no candidates (safety block -> `content_policy_violation`), text-only response, empty parts, no inline data. Checks `candidate.FinishReason` for safety blocks.
- Validates output MIME type against allowlist (`image/png`, `image/jpeg`, `image/webp`) before returning.
- Pro-model concurrency is enforced HERE (in the service layer, not middleware):
```go
// Pro models have a separate concurrency limit to prevent slow
// requests from starving fast model slots.
if modelInfo.Alias == "nano-banana-pro" {
    select {
    case client.proSemaphore <- struct{}{}:
        defer func() { <-client.proSemaphore }()
    case <-requestContext.Done():
        return nil, fmt.Errorf("%s: %s", ErrServerError, "request cancelled while waiting for pro model slot")
    }
}
```

- [ ] **Step 3: Write cancellation test in `internal/gemini/client_test.go`**

Add a test proving that blocked or in-flight requests release the Pro concurrency semaphore slot when the context is cancelled. This validates the Pro semaphore logic from Step 2.

- [ ] **Step 4: Run tests, commit**

---

## Task 9: MCP Tool Handlers

**Model:** Sonnet
**Files:**
- Create: `internal/tools/generate.go`, `edit.go`, `models.go`, `recommend.go`
- Create: `internal/tools/tools_test.go`

**Checklist:**

**Tool response contract (all 4 tools must follow this consistently):**

**Behavioral contract (normative -- must be preserved regardless of library API):**

1. **On success:** Return a text-content tool result containing the JSON-serialized result struct. The Go `error` return is always `nil`.
2. **On application-level error** (validation failure, Gemini error): Return an error-content tool result with a safe, predefined message like `"invalid_prompt: prompt is required"` or `"content_policy_violation: The prompt was blocked by content safety policy."`. Never return raw upstream error text. The Go `error` return is always `nil`.
3. **On unexpected internal error** (JSON marshal failure): Return an error-content tool result with `"server_error: internal error"`. The Go `error` return is always `nil`.
4. **Handlers never return a Go `error`** (second return value). All errors are communicated as MCP tool result errors so the MCP protocol handles them correctly.

**Expected library API (verify before implementing):** Based on `mark3labs/mcp-go` as of early 2026, the expected helpers are `mcp.NewToolResultText(string)` for success and `mcp.NewToolResultError(string)` for errors, with handler signatures like `func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)`. **Before implementing, check the installed `mcp-go` package source to confirm these names and signatures exist.** If the API has changed, adapt to match the actual exports while preserving the behavioral contract above.

Key implementation details:
- ALL handlers accept `gemini.GeminiService` interface, not `*gemini.Client`
- JSON output uses `json.Marshal` (compact, no indentation). Tests should compare decoded structs or use `json.Compact` for string comparison, not raw string equality against pretty-printed JSON.
- `edit.go` uses decoded bytes from `security.ValidateAndDecodeImage` (no double decode)
- `models.go` uses `gemini.AllModelsSafe()` (returns SafeModelInfo, no GeminiID)
- Tool handler function signatures:
```go
func NewGenerateImageHandler(service gemini.GeminiService) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
```

- **Tests must include behavioral tests using a mock:**

```go
// mockGeminiService is a test double that records calls and returns
// predefined responses, enabling handler testing without network access.
type mockGeminiService struct {
	generateResult *gemini.ImageResult
	generateError  error
	editResult     *gemini.ImageResult
	editError      error
}

func (mock *mockGeminiService) GenerateImage(requestContext context.Context, modelAlias string, prompt string, options gemini.GenerateOptions) (*gemini.ImageResult, error) {
	return mock.generateResult, mock.generateError
}

func (mock *mockGeminiService) EditImage(requestContext context.Context, modelAlias string, imageData []byte, mimeType string, instructions string) (*gemini.ImageResult, error) {
	return mock.editResult, mock.editError
}
```

Tests to include:
- Valid generate request -> success JSON
- Empty prompt -> `invalid_prompt` error
- Invalid model -> `invalid_model` error
- Gemini error -> sanitized error message returned
- list_models response has no `gemini_id` or `GeminiID` field (security test)
- recommend_model with valid/invalid priority
- edit_image with valid/invalid image

---

## Task 10: MCP Server Setup and Middleware

**Model:** Sonnet
**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/middleware.go`

> **File-size note:** `server.go` owns both `NewMCPServer` and `NewHTTPHandler`, while `middleware.go` owns all middleware logic. If either approaches 500 lines, split: e.g., tool registration into `internal/server/tools.go`, or individual middleware concerns into separate files.
- Create: `internal/server/server_test.go`

**Checklist:**

Key implementation details for `server.go` (owns both `NewMCPServer` and `NewHTTPHandler`):
- `NewMCPServer(service gemini.GeminiService) *mcpserver.MCPServer` -- creates the MCP server, registers all 4 tools (generate_image, edit_image, list_models, recommend_model) with their handler closures, returns the fully constructed server. Both transports share this instance.
- `NewHTTPHandler(mcpServer *mcpserver.MCPServer, serverConfig *config.Config, logger *slog.Logger) http.Handler` -- wraps the MCP server with HTTP routing and middleware. Details below.
- Uses `http.ServeMux` for routing
- `/healthz` and `/mcp` are both mounted on the mux
- Middleware wraps the entire mux. Inside middleware, `/healthz` is exempted by an early `next.ServeHTTP(writer, request); return` -- the mux's /healthz handler produces the response, middleware just skips its auth/rate-limit logic for that path:

```go
// NewHTTPHandler creates the HTTP handler with proper routing.
// Both /healthz and /mcp are mounted on the mux.
// Middleware wraps the entire mux. Inside middleware, /healthz is exempted
// via an early pass-through (next.ServeHTTP) -- no auth or rate limiting.
// All other paths (including /mcp) go through full auth + rate limiting.
func NewHTTPHandler(mcpServer *mcpserver.MCPServer, serverConfig *config.Config, logger *slog.Logger) http.Handler {
	serveMux := http.NewServeMux()

	// Health check -- mounted directly, no middleware
	serveMux.HandleFunc("/healthz", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		writer.Write([]byte(`{"status":"ok"}`))
	})

	// MCP endpoint -- normal JSON-RPC tool calls use POST to /mcp
	streamableServer := mcpserver.NewStreamableHTTPServer(mcpServer)
	serveMux.Handle("/mcp", streamableServer)

	// Middleware wraps all routes, but exempts /healthz by path check
	authMiddleware := NewMiddleware(serverConfig, logger)
	return authMiddleware.WrapHTTP(serveMux)
}
```

Key implementation details for `middleware.go`:
- Panic recovery with `defer recover()`
- DO NOT add a request-scoped `context.WithTimeout` to the HTTP request. SSE connections are long-lived and a hard timeout would kill them. Per-call timeouts are already enforced in the Gemini client (Task 8).
- `/healthz` is exempted by passing through to `next` without auth/rate-limit:
```go
// /healthz bypasses all middleware -- just pass through to the mux handler
if request.URL.Path == "/healthz" {
    next.ServeHTTP(writer, request)
    return
}
```
- Global concurrency semaphore queues 5 seconds before rejecting:
```go
queueTimer := time.NewTimer(5 * time.Second)
defer queueTimer.Stop()
select {
case middleware.globalSemaphore <- struct{}{}:
    defer func() { <-middleware.globalSemaphore }()
case <-queueTimer.C:
    writer.Header().Set("Content-Type", "application/json")
    writer.WriteHeader(http.StatusServiceUnavailable)
    writer.Write([]byte(`{"error":"server_busy"}`))
    return
case <-request.Context().Done():
    return
}
```
- NO `proSemaphore` in middleware (moved to gemini client service layer)
- **Oversized body enforcement strategy:** Middleware pre-reads the body up to the limit before passing to the streamable server. This guarantees the 413 contract is enforced in our code, not dependent on library internals:
  ```go
  // Pre-read the body with a size cap. This ensures oversized requests
  // are caught HERE rather than inside the streamable server library,
  // giving us control over the error response format.
  request.Body = http.MaxBytesReader(writer, request.Body, 15*1024*1024)
  bodyBytes, readError := io.ReadAll(request.Body)
  if readError != nil {
      var maxBytesError *http.MaxBytesError
      if errors.As(readError, &maxBytesError) {
          writer.Header().Set("Content-Type", "application/json")
          writer.WriteHeader(http.StatusRequestEntityTooLarge)
          writer.Write([]byte(`{"error":"request_too_large"}`))
          return
      }
      // Other read errors -- generic 400
      writer.Header().Set("Content-Type", "application/json")
      writer.WriteHeader(http.StatusBadRequest)
      writer.Write([]byte(`{"error":"bad_request"}`))
      return
  }
  // Replace the body so downstream handlers can still read it
  request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
  ```
  This approach ensures the stable JSON 413 response is always emitted by our middleware regardless of how the streamable server handles body reading internally. Tests verify the external behavior (413 + JSON body for >15MB, normal pass-through for valid sizes).
  **Tradeoff note:** Pre-reading buffers every request body into memory before handing off to the streamable server. This is an intentional bias toward stable error behavior over full streaming transparency on request bodies. It is acceptable because requests are capped at 15MB and normal MCP JSON-RPC traffic is small (typically <1KB). If future MCP protocol evolution requires true streaming request semantics, this strategy would need revisiting.

All middleware-generated error responses must use this exact pattern:
```go
writer.Header().Set("Content-Type", "application/json")
writer.WriteHeader(statusCode)
writer.Write([]byte(`{"error":"<error_key>"}`))
```

Do not use `http.Error()` for middleware responses. The JSON schema is always `{"error":"<key>"}` with no additional fields. The `Content-Type` is `application/json` (no charset suffix needed). Auth failures do not include `WWW-Authenticate` headers (the server is not a browser-facing service).

Stable error keys and status codes:
| Situation | Status | JSON body |
|---|---|---|
| Missing/wrong bearer token | 401 | `{"error":"unauthorized"}` |
| Rate limit exceeded | 429 | `{"error":"rate_limited"}` (+ `Retry-After` header) |
| Request body >15MB | 413 | `{"error":"request_too_large"}` |
| Body read error (non-size) | 400 | `{"error":"bad_request"}` |
| All concurrency slots busy | 503 | `{"error":"server_busy"}` |
| Panic recovered | 500 | `{"error":"server_error"}` |

**Middleware tests must include:**
- Correct bearer token passes through (200)
- Wrong bearer token returns 401 with `{"error":"unauthorized"}`
- Missing bearer token returns 401 with `{"error":"unauthorized"}`
- Rate limit exhaustion returns 429 with `{"error":"rate_limited"}` and `Retry-After` header
- `/healthz` bypasses auth, rate limiting, and concurrency controls via early pass-through, returns 200
- Panic in handler is recovered and returns 500 with `{"error":"server_error"}`

**Integration tests (end-to-end HTTP):**
- One test hitting `/mcp` with a `tools/list` JSON-RPC request through the full HTTP stack (with mock Gemini service)
- One test verifying an oversized request body (>15MB) returns 413 with `{"error":"request_too_large"}` and the mock Gemini service explicitly asserts zero method calls (confirming rejection happened before service invocation)
- Panic in handler is recovered and returns 500

---

## Task 11: Main Entry Point with Dual Transport

**Model:** Sonnet
**Files:**
- Modify: `cmd/mcp-banana/main.go`

**Checklist:**

Key implementation details:

**Startup order in main():**
1. Parse flags (`--transport`, `--addr`, `--healthcheck`, `--version`). Defaults: `--transport stdio` (most common use case -- Claude Code launches the binary directly), `--addr 0.0.0.0:8847`
2. If `--version`: print `"mcp-banana <version>"` where version defaults to `"dev"` and is overridable via `-ldflags="-X main.version=1.0.0"`. Exits 0. Runs before config loading or registry validation.
3. If `--healthcheck`: perform HTTP GET (with 5s client timeout) to `http://<addr>/healthz`, exit 0 on 200, exit 1 otherwise. This does NOT load config, validate registry, or create Gemini client -- it only probes an already-running server.
4. Load config (`config.Load()`)
5. If `--transport http` and no `MCP_AUTH_TOKEN`: fail fast
6. Validate registry (`gemini.ValidateRegistryAtStartup()`) -- fails with sentinel IDs
7. Register secrets with sanitizer
8. Create Gemini client
9. Create MCP server + HTTP handler
10. Start transport

**`--healthcheck` flag spec:**
```go
// --version prints the build version and exits immediately.
// Runs before config loading or registry validation.
// The version variable defaults to "dev" and can be overridden at build time:
//   go build -ldflags="-X main.version=1.0.0" ...
var version = "dev"

if *versionFlag {
    fmt.Printf("mcp-banana %s\n", version)
    os.Exit(0)
}

// --healthcheck performs a local HTTP GET to the running server's /healthz
// endpoint with a 5-second timeout. It does NOT require config loading,
// registry validation, or Gemini client initialization -- it only probes
// an already-running instance. Used by Docker HEALTHCHECK.
if *healthcheck {
    // 5-second timeout prevents hung sockets from stalling Docker health checks.
    // Default redirect behavior is acceptable -- /healthz should not redirect.
    httpClient := &http.Client{Timeout: 5 * time.Second}
    healthResponse, fetchError := httpClient.Get(fmt.Sprintf("http://%s/healthz", *address))
    if fetchError != nil {
        fmt.Fprintf(os.Stderr, "health check failed: %s\n", fetchError)
        os.Exit(1)
    }
    defer healthResponse.Body.Close()
    if healthResponse.StatusCode != http.StatusOK {
        fmt.Fprintf(os.Stderr, "health check returned status %d\n", healthResponse.StatusCode)
        os.Exit(1)
    }
    os.Exit(0)
}
```

- Default bind address: `0.0.0.0:8847` (for Docker compatibility -- NOT 127.0.0.1)
- NO `WriteTimeout` on http.Server (SSE connections are long-lived):
```go
httpServer := &http.Server{
    Addr:        *address,
    Handler:     handler,
    ReadTimeout: 30 * time.Second,
    // WriteTimeout must NOT be set for SSE/Streamable HTTP.
    // SSE connections are long-lived. Use context cancellation instead.
    IdleTimeout: 120 * time.Second,
}
```
- Auth token REQUIRED for HTTP mode:
```go
if *transport == "http" && serverConfig.AuthToken == "" {
    fmt.Fprintf(os.Stderr, "MCP_AUTH_TOKEN is required for HTTP transport mode\n")
    os.Exit(1)
}
```
- Validate model registry at startup (before creating Gemini client or accepting requests):
```go
if registryError := gemini.ValidateRegistryAtStartup(); registryError != nil {
    fmt.Fprintf(os.Stderr, "model registry validation failed: %s\n", registryError)
    os.Exit(1)
}
```
- Register BOTH API key and auth token with sanitizer:
```go
security.RegisterSecret(serverConfig.GeminiAPIKey)
if serverConfig.AuthToken != "" {
    security.RegisterSecret(serverConfig.AuthToken)
}
```
- Graceful shutdown with 120s timeout (HTTP mode only -- see below)
- Use `server.NewHTTPHandler()` from Task 10 (not manual mux setup)

**Tool registration and server construction:**
- Tool registration lives in `internal/server/` (Task 10). A factory function `server.NewMCPServer(service gemini.GeminiService) *mcpserver.MCPServer` creates the MCP server instance, registers all 4 tools (generate_image, edit_image, list_models, recommend_model) with their handlers, and returns the fully constructed server.
- Both stdio and HTTP transports share the same `*mcpserver.MCPServer` instance -- tools are registered once, transport is chosen at runtime.
- `main.go` calls `server.NewMCPServer(geminiService)` and then passes the result to either `ServeStdio` or `NewHTTPHandler` based on the `--transport` flag.

**stdio transport specifics:**
- Use `mcpserver.ServeStdio(mcpServer)` from `mark3labs/mcp-go/server`. This blocks until stdin is closed (i.e., Claude Code terminates the process).
- No graceful shutdown handler is needed for stdio -- the process exits when the parent (Claude Code) closes stdin. Signal handling is for HTTP mode only.
- No middleware, no auth, no rate limiting in stdio mode. Security relies on OS process isolation.

**HTTP transport specifics:**
- Use `server.NewHTTPHandler(mcpServer, serverConfig)` from Task 10, which wraps the shared MCP server with ServeMux, middleware, and Streamable HTTP transport.
- Graceful shutdown: goroutine listens for SIGTERM/SIGINT, calls `httpServer.Shutdown(context)` with 120s timeout.
- All middleware (auth, rate limiting, concurrency, panic recovery) applies to HTTP mode only.

---

## Task 12: Dockerfile and Docker Compose

**Model:** Haiku
**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`

Key details:
- CMD binds `0.0.0.0:8847` (inside container, needs all interfaces for Docker port forwarding)
- HEALTHCHECK targets `127.0.0.1:8847` (self-check inside container):
```dockerfile
CMD ["--transport", "http", "--addr", "0.0.0.0:8847"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/mcp-banana", "--healthcheck", "--addr", "127.0.0.1:8847"]
```
- docker-compose.yml port mapping restricts host exposure: `127.0.0.1:8847:8847`
- `stop_grace_period: 120s`
- `mem_limit: 768m`
- No vendor/ in .dockerignore (we're not vendoring)

> **Development note:** While sentinel model IDs are present, the container will fail to become healthy because `ValidateRegistryAtStartup()` intentionally blocks boot. Depending on Docker's restart policy and timing, the container may exit immediately, restart repeatedly, or be marked as unhealthy. This is expected behavior during development, not a Docker bug. The container will only become healthy after sentinel IDs are replaced with verified Gemini model IDs.

---

## Task 13: README

**Model:** Haiku -- Write comprehensive README with architecture overview, text-based architecture diagram, threat model section (not "brief"), security model explanation, setup guide, deployment instructions.

The README must explicitly state:
- `nano-banana-original` is **provisional** and may be removed before release if no verified Gemini model ID exists for it. `nano-banana-2` and `nano-banana-pro` are the intended verified targets based on current Google documentation, but the registry remains release-blocked until the exact IDs are checked live and the sentinels are replaced.
- Normal JSON-RPC tool calls use POST to `/mcp`.
- The README should include the HTTP error contract table (401 unauthorized, 413 request_too_large, 400 bad_request, 429 rate_limited, 503 server_busy, 500 server_error) so API clients know the stable error keys.
- The server is implementation-ready but release-blocked until model IDs in `internal/gemini/registry.go` are verified against the live Gemini API.
- **Before model IDs are verified, the server will intentionally refuse to start.** This is by design -- `ValidateRegistryAtStartup()` blocks boot when sentinel IDs are present. The Docker container will fail to become healthy (it may exit, restart, or remain unhealthy depending on restart policy). This is expected behavior, not a bug. Replace all `VERIFY_MODEL_ID_BEFORE_RELEASE` values with verified Gemini model IDs to enable runtime.
- Include a **Go Glossary** section for contributors unfamiliar with Go, explaining the standard abbreviations used in the codebase:
  - `err` -- holds the error returned by the previous operation (Go functions return errors as values, not exceptions)
  - `ctx` -- carries request-scoped deadlines, cancellation signals, and metadata across API boundaries
  - `req` -- the incoming HTTP request from the client
  - `resp` -- the HTTP response received from an upstream service
  - `cfg` -- the parsed application configuration (from environment variables in this project)
  - `srv` -- the HTTP server instance that listens for incoming connections
  - `test` -- the Go test runner (`*testing.T`) that provides logging and failure reporting
- Include a **Prerequisites** section mentioning: Go 1.24+, `golangci-lint`, Docker (for deployment), SSH access (for DigitalOcean deployment)

---

## Task 14: Quality Gate

**Model:** Haiku

> **Note:** The local quality gate intentionally does not include Docker build/run checks. CI (Task 15) includes a Docker build-only check. Docker runtime verification is deferred to release mode after sentinel IDs are replaced.

**Checklist:**

- [ ] **Step 1: Run golangci-lint**
```bash
golangci-lint run
```

- [ ] **Step 2: Format and verify**
```bash
gofmt -w .
make fmt-check
```
Note: `gofmt -w .` applies formatting fixes. `make fmt-check` then verifies no files remain unformatted. The Makefile `quality-gate` target uses `fmt-check` (verify-only) to keep the gate purely diagnostic. Use `make fmt` for manual formatting during development.

- [ ] **Step 3: Run go vet**
```bash
go vet ./...
```

- [ ] **Step 4: Run tests with coverage and race detector**
```bash
go test -coverprofile=coverage.out -race ./... -v
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
echo "Total coverage: ${COVERAGE}%"
awk -v cov="$COVERAGE" 'BEGIN { if (cov < 80.0) { print "Coverage "cov"% is below 80% threshold"; exit 1 } }'
```

- [ ] **Step 5: Verify build**
```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o mcp-banana ./cmd/mcp-banana/
ls -lh mcp-banana
```
Expected: binary < 15MB

- [ ] **Step 6: Final commit**
```bash
git add -A
git reset .claude/settings.local.json 2>/dev/null || true
if git diff --cached --name-only | grep -q '^\.claude/settings\.local\.json$'; then
  echo "ERROR: .claude/settings.local.json is staged"
  exit 1
fi
git commit -m "chore: pass quality gate with coverage and race detection"
```

---

## Task 15: GitHub Actions CI Workflow

**Model:** Sonnet
**Files:**
- Create: `.github/workflows/ci.yml`

This workflow runs on pushes to `feat/**`, `fix/**`, and `chore/**` branches, plus pull requests targeting `main`. For pushes to `main`, CI runs via the CD workflow's `workflow_call` invocation (avoiding duplicate runs). It also supports direct reusable invocation via `workflow_call`. It mirrors the local quality gate in an automated environment. CI must pass before any merge or deploy.

> **Action pinning:** All third-party GitHub Actions are pinned to commit SHAs (not version tags) for supply chain security. The golangci-lint binary version is intentionally fixed to `v2.1.6` for reproducibility. If a newer version is needed later, update both the action SHA and the `version:` input together.

**Checklist:**

- [ ] **Step 1: Create `.github/workflows/ci.yml`**

```yaml
name: CI

on:
  push:
    # main is excluded here because cd.yml already invokes ci.yml via
    # workflow_call on pushes to main, avoiding duplicate CI runs.
    branches:
      - 'feat/**'
      - 'fix/**'
      - 'chore/**'
  pull_request:
    branches: [main]
  # Support reusable invocation from CD workflow (runs on main pushes)
  workflow_call:

permissions:
  contents: read

jobs:
  ci:
    name: Lint, Test, Build
    runs-on: ubuntu-latest
    timeout-minutes: 15

    steps:
      # Pin all actions to commit SHAs for supply chain security.
      # SHAs are pinned. Update together when intentionally upgrading actions.
      - name: Checkout code
        uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2

      - name: Set up Go
        uses: actions/setup-go@d35c59abb061a4a6fb18e82ac0862c26744d6ab5 # v5.5.0
        with:
          go-version: '1.24'
          cache: true

      - name: Install and run golangci-lint
        uses: golangci/golangci-lint-action@4afd733a84b1f43292c63897423277bb7f4313a9 # v6.5.0
        with:
          version: v2.1.6
          args: --timeout=5m

      - name: Check formatting
        run: |
          gofmt -l .
          test -z "$(gofmt -l .)" || (echo "formatting issues found" && exit 1)

      - name: Run go vet
        run: go vet ./...

      - name: Run tests with coverage and race detector
        run: |
          go test -coverprofile=coverage.out -race ./... -v
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
          echo "Total coverage: ${COVERAGE}%"
          awk -v cov="$COVERAGE" 'BEGIN { if (cov < 80.0) { print "Coverage "cov"% is below 80% threshold"; exit 1 } }'

      - name: Upload coverage artifact
        uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02 # v4.6.2
        with:
          name: coverage-report
          path: coverage.out
          retention-days: 7

      - name: Build production binary
        run: |
          CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
            -ldflags="-s -w" \
            -trimpath \
            -o mcp-banana \
            ./cmd/mcp-banana/

      - name: Verify binary size
        run: |
          SIZE=$(stat -c%s mcp-banana 2>/dev/null || stat -f%z mcp-banana)
          echo "Binary size: $SIZE bytes"
          MAX_SIZE=$((15 * 1024 * 1024))
          if [ "$SIZE" -gt "$MAX_SIZE" ]; then
            echo "Binary exceeds 15MB limit"
            exit 1
          fi

      - name: Build Docker image (build-only, do not run -- startup fails with sentinel IDs)
        run: docker build -t mcp-banana:ci .

      - name: Verify Docker image size
        # Image size depends on the Dockerfile from Task 12 (base image + binary).
        # Expected ~14MB with distroless + stripped Go binary. 25MB budget provides headroom.
        run: |
          SIZE=$(docker image inspect mcp-banana:ci --format='{{.Size}}')
          echo "Docker image size: $SIZE bytes"
          MAX_SIZE=$((25 * 1024 * 1024))
          if [ "$SIZE" -gt "$MAX_SIZE" ]; then
            echo "Docker image exceeds 25MB limit"
            exit 1
          fi
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add GitHub Actions CI workflow with lint, test, build, and Docker verification"
```

---

## Task 16: GitHub Actions CD Workflow

**Model:** Sonnet
**Files:**
- Create: `.github/workflows/cd.yml`

This workflow runs only on pushes/merges to `main`, only after CI passes (via reusable `workflow_call`). It deploys to the DigitalOcean droplet via a single consolidated SSH step that handles deploy, health check, and auto-rollback atomically. If the new container fails the health check, it automatically reverts to the previous version before failing the GitHub Action.

**Checklist:**

- [ ] **Step 1: Create `.github/workflows/cd.yml`**

```yaml
name: CD

on:
  push:
    branches: [main]

permissions:
  contents: read

# Ensure only one deployment runs at a time
concurrency:
  group: deploy-production
  cancel-in-progress: false

jobs:
  # CI must pass first -- reuse ci.yml via workflow_call
  ci:
    uses: ./.github/workflows/ci.yml

  deploy:
    name: Deploy to DigitalOcean
    needs: ci
    runs-on: ubuntu-latest
    timeout-minutes: 10
    environment: production

    steps:
      - name: Check for sentinel model IDs
        uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
        # Guard: block deployment if sentinel IDs are still present.
        # This prevents deploying a build that will fail on startup.
      - name: Block deploy if model IDs unverified
        run: |
          if grep -r "VERIFY_MODEL_ID_BEFORE_RELEASE" internal/gemini/registry.go; then
            echo "DEPLOYMENT BLOCKED: Sentinel model IDs still present in registry."
            echo "Replace all VERIFY_MODEL_ID_BEFORE_RELEASE values with verified Gemini model IDs before deploying."
            exit 1
          fi

      # Pin action to commit SHA for supply chain security.
      # SHA is pinned. Update when intentionally upgrading this action.
      - name: Deploy, verify, and auto-rollback on failure
        uses: appleboy/ssh-action@2ead5e36573ebcdced20eb94e652a0a18e2e5745 # v1.2.2
        with:
          host: ${{ secrets.DEPLOY_HOST }}
          username: ${{ secrets.DEPLOY_USER }}
          key: ${{ secrets.DEPLOY_SSH_KEY }}
          script_stop: true
          script: |
            # -u: fail on unset variables. -o pipefail: fail on pipe errors.
            # -e is deliberately omitted: each critical command is guarded with
            # explicit `if !` checks that trigger the rollback function. Using -e
            # would exit before rollback could run.
            set -uo pipefail
            cd /opt/mcp-banana
            PREV_SHA=$(git rev-parse HEAD)
            echo "Current SHA: $PREV_SHA"

            # Rollback function used by all failure paths
            rollback() {
              echo "Auto-rolling back to $PREV_SHA..."
              git reset --hard "$PREV_SHA"
              docker compose up -d --build --force-recreate
              sleep 10
              if curl -sf http://127.0.0.1:8847/healthz > /dev/null 2>&1; then
                echo "Rollback successful -- server is back on $PREV_SHA"
              else
                echo "WARNING: Rollback container also failed. Manual intervention required."
              fi
              echo "Deployment of $NEW_SHA failed. Server is running $PREV_SHA."
            }

            echo "Fetching new code..."
            if ! git fetch origin main; then
              echo "git fetch failed"
              NEW_SHA="(fetch failed)"
              rollback
              exit 1
            fi

            if ! git reset --hard origin/main; then
              echo "git reset failed"
              NEW_SHA="(reset failed)"
              rollback
              exit 1
            fi

            NEW_SHA=$(git rev-parse HEAD)
            echo "New SHA: $NEW_SHA"

            echo "Building and starting new container..."
            if ! docker compose up -d --build --force-recreate; then
              echo "docker compose build/start failed"
              rollback
              exit 1
            fi

            echo "Waiting for health check (30s max)..."
            for attempt in 1 2 3 4 5 6; do
              if curl -sf http://127.0.0.1:8847/healthz > /dev/null 2>&1; then
                HEALTH_RESPONSE=$(curl -sf http://127.0.0.1:8847/healthz)
                if [ "$HEALTH_RESPONSE" = '{"status":"ok"}' ]; then
                  echo "Deploy successful! Health check passed on attempt $attempt"
                  docker image prune -f
                  exit 0
                fi
              fi
              echo "Attempt $attempt: not ready, waiting 5s..."
              sleep 5
            done

            echo "Health check FAILED after 30 seconds!"
            rollback
            exit 1
```

> **Rollback behavior:** The deploy, health check, and rollback are consolidated into a single SSH step so the server is never left in a broken state. If the new container fails the health check, the script automatically reverts the git state AND rebuilds the previous container before failing the GitHub Action. This minimizes downtime and MTTR.

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/cd.yml
git commit -m "cd: add GitHub Actions CD workflow with deploy, smoke test, and auto-rollback"
```

---

## Task 17: GitHub Actions Secrets and Deployment Configuration

**Model:** Haiku
**Files:** None (GitHub UI configuration only)

> **Human-only task.** Steps 1-3 require GitHub web UI access and cannot be executed by a coding agent. The agent should skip to Step 4 (README documentation) and Step 5 (commit). Steps 1-3 must be completed manually by the developer.

This task configures the GitHub repository secrets and environments needed by the CI/CD workflows. No code changes -- this is infrastructure setup.

**Checklist:**

- [ ] **Step 1: Create GitHub repository environment `production`**

In the GitHub repository Settings > Environments, create an environment named `production` with:
- Required reviewers (optional but recommended for deploy approval)
- Deployment branch restriction: `main` only

Additionally, configure branch protection on `main`:
- Require the CI status check to pass before merge (note: the check must have run successfully at least once in the repo within the last 7 days to be selectable in branch protection settings)
- Require pull request reviews before merging (recommended)

- [ ] **Step 2: Configure repository secrets**

In GitHub repository Settings > Environments > `production` > Environment secrets, add these as **environment secrets** (not repository secrets). Environment secrets are only accessible to jobs that reference the `production` environment, which enforces the branch restriction and protection rules:

| Secret Name | Value | Used By |
|---|---|---|
| `DEPLOY_HOST` | DigitalOcean droplet IP address | CD workflow SSH |
| `DEPLOY_USER` | SSH username on the droplet | CD workflow SSH |
| `DEPLOY_SSH_KEY` | Private SSH key for deployment (ed25519 recommended) | CD workflow SSH |

> **SECURITY:** These are environment secrets scoped to the `production` environment. They are only accessible to jobs that declare `environment: production`, which requires the branch restriction and protection rules to pass first. They are NEVER logged or exposed in workflow output.

> **Note:** `GEMINI_API_KEY` and `MCP_AUTH_TOKEN` are NOT stored in GitHub Actions. They live only on the droplet in `/opt/mcp-banana/.env`. The CI/CD pipeline never needs them -- CI runs tests with mocks, and CD deploys code that reads secrets from the server's environment at runtime.

- [ ] **Step 3: Verify the droplet is configured for deployment**

On the DigitalOcean droplet:
```bash
# Ensure the deploy directory exists and is a git repo
ls -la /opt/mcp-banana/.env
ls -la /opt/mcp-banana/docker-compose.yml

# Ensure Docker is installed and running
docker compose version

# Ensure the deploy user can run docker commands
docker ps
```

- [ ] **Step 4: Document the CI/CD setup in README**

Add a CI/CD section to the README covering:
- CI runs on pushes to feature/fix/chore branches and on pull requests to main. On pushes to main, CI is invoked by the CD workflow before deployment.
- CD runs on pushes to main (deploy to DigitalOcean, smoke test)
- Deployment secrets are stored as GitHub environment secrets, not in code
- Application runtime secrets (API key, auth token) live only on the server
- Rollback is automatic when post-deploy health checks fail; manual intervention is required only if the rollback itself also fails

- [ ] **Step 5: Commit any documentation updates**

```bash
git add README.md
git commit -m "docs: add CI/CD pipeline documentation to README"
```

---

## Task 18: Claude Code Integration Documentation

**Model:** Haiku
**Files:**
- Modify: `README.md` (add "Using with Claude Code" section)

This task creates end-user documentation for connecting Claude Code to the mcp-banana server using Claude Code's native MCP management commands (`claude mcp add`, `claude mcp list`, etc.), not manual JSON editing.

**Checklist:**

- [ ] **Step 1: Add "Using with Claude Code" section to README**

The README section must include the following subsections:

---

**Team adoption recommendation:**

| Scope | Use case | How |
|---|---|---|
| **User scope** (`--scope user`) | Personal local development. API key stays private to your machine. | `claude mcp add --scope user ...` (stored in `~/.claude.json`) |
| **Project scope** (`--scope project`) | Shared team configuration checked into version control. | `claude mcp add --scope project ...` (stored in `.mcp.json`) |
| **HTTP** | Preferred remote transport for deployed servers. | Direct HTTP or SSH-tunneled HTTP |
| **SSE** | Deprecated. HTTP is the replacement for all new Claude Code setups. | Do not use |

---

**Option A: Local stdio mode (recommended for development)**

Prerequisites:
- Build and install the binary: `go build -o /usr/local/bin/mcp-banana ./cmd/mcp-banana/`
- Obtain a Gemini API key from https://aistudio.google.com/

User-scoped setup (personal, absolute path, not committed to repo):
```bash
claude mcp add --scope user banana \
  --transport stdio \
  -- /usr/local/bin/mcp-banana --transport stdio
```

Then set the API key environment variable. Use `claude mcp add-json` for full control:
```bash
claude mcp add-json --scope user banana '{
  "command": "/usr/local/bin/mcp-banana",
  "args": ["--transport", "stdio"],
  "env": {
    "GEMINI_API_KEY": "<your-gemini-api-key>"
  },
  "type": "stdio"
}'
```

Project-scoped setup (shared team config, env-expanded command, committed to repo):
```bash
claude mcp add-json --scope project banana '{
  "command": "${MCP_BANANA_BIN:-mcp-banana}",
  "args": ["--transport", "stdio"],
  "type": "stdio"
}'
```

> **Path convention:** Project-scoped `.mcp.json` uses `command: "${MCP_BANANA_BIN:-mcp-banana}"`, which supports environment variable expansion. This defaults to looking up `mcp-banana` on `$PATH`, but team members can override by setting `MCP_BANANA_BIN` to a custom path. User-scoped config in `~/.claude.json` uses an absolute path (`/usr/local/bin/mcp-banana`) for explicitness.

> **IMPORTANT: No secrets in project-scoped config.** The `.mcp.json` file is committed to version control and must NEVER contain API keys or tokens. Each developer supplies their own `GEMINI_API_KEY` by adding a user-scoped override:
> ```bash
> claude mcp add-json --scope user banana '{
>   "command": "/usr/local/bin/mcp-banana",
>   "args": ["--transport", "stdio"],
>   "env": {
>     "GEMINI_API_KEY": "<your-personal-gemini-api-key>"
>   },
>   "type": "stdio"
> }'
> ```
> Each developer should supply secrets through their own user-scoped config or local environment, while the committed project config remains secret-free. The team shares the base server definition; individual developers provide their own credentials.

> **Project-scoped approval behavior:** When a team member first opens a project with a `.mcp.json` file, Claude Code prompts them to review and approve each configured MCP server before it runs. This is a security measure -- the developer explicitly trusts the server configuration before it can execute on their machine.

How it works:
- Claude Code spawns the mcp-banana binary as a child process
- Communication happens over stdin/stdout using JSON-RPC
- The API key is passed as an environment variable and is never visible in tool responses
- No network ports are opened, significantly reducing network exposure

> **Pre-release note:** The server will refuse to start until sentinel model IDs in `internal/gemini/registry.go` are replaced with verified Gemini model IDs. This applies to both stdio and HTTP modes. See the README's release-blocked note for details.

---

**Option B: Remote HTTP mode (for deployed server)**

Prerequisites:
- mcp-banana deployed on DigitalOcean droplet (see deployment section)
- `MCP_AUTH_TOKEN` set on the server (in `/opt/mcp-banana/.env`)

Direct HTTP setup (server accessible via tunnel or private network):
```bash
claude mcp add-json --scope user banana '{
  "type": "http",
  "url": "http://localhost:8847/mcp",
  "headers": {
    "Authorization": "Bearer <your-mcp-auth-token>"
  }
}'
```

> **Note on transport type:** Claude Code users configure the remote server as `"type": "http"`. The server implementation uses Streamable HTTP (MCP protocol spec) under the hood, but the user-facing Claude Code config type is simply `"http"`.

**Optional hardening: SSH tunnel**

If the server is deployed on a non-public droplet (recommended), use an SSH tunnel to avoid exposing any port publicly:

```bash
# Open tunnel (run once per session, or use autossh for persistence)
ssh -N -L 8847:127.0.0.1:8847 user@<droplet-ip>
```

With the tunnel active, `http://localhost:8847/mcp` in the Claude Code config reaches the remote server securely. The SSH tunnel provides transport encryption and authentication. The bearer token provides application-layer authentication as defense-in-depth.

Without the tunnel, the server would need to be exposed on a public port with TLS termination -- SSH tunneling avoids that complexity entirely.

> **Pre-release note:** HTTP mode also requires verified model IDs. The server will refuse to start with sentinel IDs, so remote deployment is only functional after the release gate is cleared.

---

**Verification -- confirm Claude Code can connect:**

> These verification steps apply only after verified model IDs replace the sentinel values. With sentinel IDs, the server will refuse to start and Claude Code will not be able to connect.

Using Claude Code's native MCP management commands:

```bash
# List all configured MCP servers
claude mcp list

# Get details for the banana server
claude mcp get banana
```

Then verify tool discovery:

1. Ask Claude Code: "What image generation tools are available?"
   - Expected: Claude Code discovers and lists the 4 tools (generate_image, edit_image, list_models, recommend_model)
2. Ask Claude Code: "List the available Nano Banana models"
   - Expected: Claude Code calls `list_models` and shows the 3 model aliases
3. Ask Claude Code: "Recommend a model for creating a product photo"
   - Expected: Claude Code calls `recommend_model` and returns a valid recommendation with a model alias and reason (the specific model depends on the recommendation policy)

---

**Troubleshooting:**

| Symptom | Cause | Fix |
|---|---|---|
| `claude mcp list` does not show banana | Server not added or wrong scope | Re-run `claude mcp add-json` with correct `--scope` |
| Claude Code says "server not found" | Binary path wrong or not built | Verify: `which mcp-banana` or check the path in `claude mcp get banana` |
| "configuration error: GEMINI_API_KEY is required" | API key not set in env config | Re-add with `claude mcp add-json` including the `env` block |
| "model registry validation failed" | Sentinel model IDs still present | Replace `VERIFY_MODEL_ID_BEFORE_RELEASE` in `internal/gemini/registry.go` with verified Gemini model IDs |
| Server starts but tools return errors | API key invalid or expired | Verify key at https://aistudio.google.com/ |
| HTTP mode: "unauthorized" | Wrong or missing bearer token | Verify `MCP_AUTH_TOKEN` in server `.env` matches the `Authorization` header in config |
| HTTP mode: connection refused | SSH tunnel not running or server down | Start tunnel: `ssh -N -L 8847:127.0.0.1:8847 user@<droplet-ip>`. Check server: `docker compose ps` |
| Tools discovered but generation fails | Gemini API quota exceeded or model unavailable | Check server logs: `docker compose logs -f` |
| Project-scoped server prompts for approval | Normal behavior for `.mcp.json` servers | Approve the server when prompted on first use |

---

- [ ] **Step 2: Commit the project-scoped `.mcp.json` (secret-free)**

The project-scoped `.mcp.json` is committed directly to the repo. It contains only the server command and transport -- NO secrets:
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

**Repo policy:** This file is committed intentionally. It must remain secret-free at all times -- no API keys, tokens, or credentials. Personal credentials belong only in user-scoped config (`~/.claude.json`).

> **Note on binary path:** The project-scoped config uses `${MCP_BANANA_BIN:-mcp-banana}`, which defaults to looking up `mcp-banana` on `$PATH`. To override, set the `MCP_BANANA_BIN` environment variable to your binary's location (e.g., `export MCP_BANANA_BIN=/opt/bin/mcp-banana`).

- [ ] **Step 3: Commit**

```bash
git add README.md .mcp.json
git commit -m "docs: add Claude Code integration guide with native MCP commands"
```

---

## Post-Implementation Verification

### Development verification (while sentinel IDs are present)

> **Note:** With sentinel model IDs, `ValidateRegistryAtStartup()` will cause the server to exit immediately on startup. This is intentional. The following checks validate the build and test suite without requiring a running server.

1. **Build compiles cleanly:**
   ```bash
   CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o mcp-banana ./cmd/mcp-banana/
   ```

2. **All unit and integration tests pass.** The sentinel test asserts startup rejection is working. No tests require successful server startup with sentinel IDs -- runtime transport verification is deferred to release mode.
   ```bash
   go test -coverprofile=coverage.out -race ./... -v
   ```

3. **Version flag works:**
   ```bash
   ./mcp-banana --version
   ```

4. **Startup correctly rejects sentinel IDs:**
   ```bash
   GEMINI_API_KEY=test ./mcp-banana --transport stdio 2>&1
   # Expected: "model registry validation failed" error and exit code 1
   ```

### Release verification (after replacing sentinel IDs with verified Gemini model IDs)

1. **stdio test:**
   ```bash
   echo '{"jsonrpc":"2.0","method":"tools/list","id":1}' | GEMINI_API_KEY=<real-key> ./mcp-banana --transport stdio
   ```
   Should return JSON with 4 tools listed.

2. **HTTP test** (requires GEMINI_API_KEY and MCP_AUTH_TOKEN in .env):
   ```bash
   source .env
   ./mcp-banana --transport http &
   curl -s http://127.0.0.1:8847/healthz
   # Should return {"status":"ok"}
   curl -s -X POST \
     -H "Authorization: Bearer $MCP_AUTH_TOKEN" \
     -H "Content-Type: application/json" \
     http://127.0.0.1:8847/mcp \
     -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
   kill %1
   ```

3. **Docker test:**
   ```bash
   docker compose up -d --build
   curl -s http://127.0.0.1:8847/healthz
   docker compose down
   ```

### CI/CD verification

1. **CI workflow triggers on push:**
   Push a branch and verify the CI workflow runs in GitHub Actions. All steps (lint, format, vet, test, build, Docker build) should pass.

2. **CI blocks PRs on failure:**
   Create a PR to `main`. Verify the CI check appears as a required status check.

3. **CD workflow triggers on merge to main:**
   After merging a PR to `main`, verify the CD workflow runs: deploy via SSH, health check, smoke test.

4. **Rollback behavior on failure:**
   If the post-deploy health check fails, verify the deployment automatically reverts to the previous SHA and rebuilds the old container. Manual intervention is required only if the rollback health check also fails.

---

## Summary: Task Execution Order

| Task | Description | Model | Key Fixes Inlined |
|---|---|---|---|
| 0 | Optimize .claude/ config | Sonnet | One-time plugin normalization, Go-only rules, remove irrelevant scaffold content |
| 1 | Project scaffolding | Haiku | .golangci.yml, .gitattributes, no vendor |
| 2 | Configuration loading | Haiku | Configurable limits, invalid log level test |
| 3 | Model registry | Haiku | SafeModelInfo DTO, sentinel IDs, sorted output |
| 4 | Gemini error handling | Sonnet | genai.APIError (not googleapi), HTTP status tests |
| 5 | Input validation | Sonnet | Rune length, ValidateAndDecodeImage, magic bytes |
| 6 | Output sanitization | Sonnet | ClearSecrets, test isolation |
| 7 | Model recommendation | Sonnet | (unchanged) |
| 8 | Gemini client wrapper | Sonnet | GeminiService interface, per-call timeout, Pro concurrency, safety block handling |
| 9 | MCP tool handlers | Sonnet | Interface-based, mock tests, no double decode |
| 10 | Server + middleware | Sonnet | ServeMux routing, panic recovery, queue before 503, /healthz exemption |
| 11 | Main entry point | Sonnet | No WriteTimeout, 0.0.0.0 bind, auth required, registry validation, register both secrets |
| 12 | Dockerfile + compose | Haiku | 0.0.0.0 CMD, 127.0.0.1 healthcheck |
| 13 | README | Haiku | Threat model section, architecture diagram |
| 14 | Quality gate | Haiku | Coverage + race detection |
| 15 | GitHub Actions CI | Sonnet | Lint, test, build, Docker build on every push/PR |
| 16 | GitHub Actions CD | Sonnet | Consolidated deploy+healthcheck+auto-rollback in single SSH step |
| 17 | Deployment secrets | Haiku | GitHub environments, SSH keys, README CI/CD docs |
| 18 | Claude Code integration | Haiku | Stdio + HTTP setup, config examples, verification, troubleshooting |

Execute sequentially. Commit after each task. Do not parallelize.
