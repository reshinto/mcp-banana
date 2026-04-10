# mcp-banana Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Comprehensive project documentation with Mermaid PNG diagrams, Go language guide, authentication guide, troubleshooting, and end-to-end flow walkthrough.

**Architecture:** Documentation under `docs/` with PNG diagrams in `docs/diagrams/`. Mermaid `.mmd` sources kept alongside rendered `.png` files. Root README is a concise landing page linking to all docs.

**Tech Stack:** Mermaid CLI (`mmdc`), Markdown, PNG

**Status:** COMPLETE -- all tasks implemented.

---

## Deduplication Strategy

Each topic has one canonical location. Other docs link to it.

| Topic | Canonical Location |
|---|---|
| Prerequisites | docs/setup-and-operations.md |
| Go Glossary | docs/go-guide.md (Section 16) |
| Model aliases table | docs/models.md |
| Error codes | docs/security.md |
| Middleware chain | docs/architecture.md |
| Auth guide | docs/authentication.md |
| Troubleshooting | docs/troubleshooting.md |

---

## Final File Structure

```
README.md                          (49 lines)   -- landing page with doc links
CONTRIBUTING.md                    (103 lines)  -- coding standards, branch workflow, PR process
docs/
  architecture.md                  (94 lines)   -- system design, package layout, 6 diagrams
  authentication.md                (221 lines)  -- SSH tunnel, single token, per-user tokens file
  claude-code-integration.md       (121 lines)  -- stdio/HTTP setup, troubleshooting
  go-guide.md                      (451 lines)  -- 16 Go concepts with project examples + glossary
  models.md                        (101 lines)  -- model aliases, sentinel verification, Docker health
  root-files.md                    (160 lines)  -- every root-level file explained
  security.md                      (125 lines)  -- threat model, validation table, error mapping
  setup-and-operations.md          (392 lines)  -- config, local/Docker setup, CI, E2E flow
  testing.md                       (186 lines)  -- test inventory, patterns, coverage
  tools-reference.md               (259 lines)  -- 4 MCP tool schemas, params, responses
  troubleshooting.md               (91 lines)   -- 10 common problems with fixes
  diagrams/                        (16 files)   -- 8 .mmd sources + 8 .png renders
    high-level-architecture.mmd + .png
    package-dependencies.mmd + .png
    request-flow.mmd + .png
    security-boundaries.mmd + .png
    startup-sequence.mmd + .png
    middleware-chain.mmd + .png
    ci-cd-pipeline.mmd + .png
    model-recommendation.mmd + .png
```

---

## Task 0: Mermaid Diagrams

- [x] Create `docs/diagrams/` directory
- [x] Create 8 `.mmd` source files (high-level-architecture, package-dependencies, request-flow, security-boundaries, startup-sequence, middleware-chain, ci-cd-pipeline, model-recommendation)
- [x] Render all to PNG: `mmdc -i <file>.mmd -o <file>.png -t neutral --backgroundColor white -s 2`
- [x] Commit diagrams

## Task 1: .env.example Documentation

- [x] Rewrite `.env.example` with inline comments for every variable
- [x] Include: what it is, how to get the value, constraints, security notes
- [x] Add example values for API key, token, and tokens file path
- [x] Commit

## Task 2: docs/architecture.md

- [x] System overview
- [x] Embed diagrams: high-level-architecture, package-dependencies, request-flow, startup-sequence, security-boundaries, middleware-chain
- [x] Package table with path, responsibility, key exports
- [x] Canonical location for middleware chain description
- [x] Link to security.md for threat model details
- [x] Commit

## Task 3: docs/go-guide.md

- [x] 16 sections covering every Go concept used in the project
- [x] Code examples from actual codebase (not generic examples)
- [x] Sections: packages/imports, structs/types, functions/methods, error handling, interfaces/DI, closures, context, goroutines/channels, defer, sync primitives, maps/slices, string/encoding, HTTP server, flags/config, testing, Go glossary
- [x] Canonical location for Go Glossary (err, ctx, req, resp, cfg, srv, test)
- [x] Commit

## Task 4: docs/security.md

- [x] Secret isolation (RegisterSecret, SanitizeString, API key pattern redaction)
- [x] Input validation table with all 6 validators and exact constraints
- [x] Error mapping boundary (MapError, 5 safe error codes, raw SDK errors never forwarded)
- [x] HTTP error contract table (401, 429, 413, 400, 503, 500)
- [x] Threat model (trust boundaries, what is protected)
- [x] Canonical location for error codes
- [x] Link to architecture.md for middleware chain
- [x] Commit

## Task 5: docs/authentication.md

- [x] Overview table of 3 auth approaches
- [x] Option 1: SSH tunnel -- admin setup (create users, add SSH keys), user setup (open tunnel, autossh, SSH config), Claude Code config, revoking access
- [x] Option 2: Single shared token -- generate, configure server and client, limitations
- [x] Option 3: Per-user tokens file -- create file, generate per-user tokens, hot-reload, add/revoke/rotate users without restart
- [x] How auth works (middleware flow)
- [x] Security properties
- [x] Commit

## Task 6: docs/setup-and-operations.md

- [x] Prerequisites table
- [x] Local development setup (5 steps)
- [x] Configuration reference table (all 9 env vars with validation rules)
- [x] Production deployment (Docker on DigitalOcean, 8 steps)
- [x] Common startup failure table
- [x] Updating production (manual via SSH)
- [x] CI pipeline description
- [x] Monitoring (health endpoint, logs, log levels)
- [x] Token rotation
- [x] End-to-end flow: prompt to image received (9 steps with source file references)
- [x] Error flow table
- [x] Commit

## Task 7: docs/tools-reference.md

- [x] 4 tool specifications (generate_image, edit_image, list_models, recommend_model)
- [x] Each tool: parameters table, validation rules, success response example, error response format
- [x] Embed model-recommendation diagram for recommend_model
- [x] Link to models.md for model details, security.md for error codes
- [x] Note: server requests image/png but Gemini may return other MIME types
- [x] Commit

## Task 8: docs/models.md

- [x] Model aliases table (nano-banana-2, nano-banana-pro, nano-banana-original)
- [x] Provisional status note for nano-banana-original
- [x] Embed model-recommendation diagram
- [x] Sentinel model ID verification procedure
- [x] ValidateRegistryAtStartup behavior and error message
- [x] Docker health implications
- [x] Commit

## Task 9: docs/testing.md

- [x] How to run tests (make test, quality gate, single package)
- [x] Coverage threshold (80%, enforced in CI)
- [x] Test inventory table (9 files, ~97 tests)
- [x] Testing patterns: dependency injection, env isolation (test.Setenv), table-driven tests, httptest, security tests
- [x] Commit

## Task 10: docs/claude-code-integration.md

- [x] Team adoption table (user scope, project scope, HTTP, SSE deprecated)
- [x] Option A: Local stdio mode (user-scoped + project-scoped)
- [x] Option B: Remote HTTP mode (link to authentication.md for auth details)
- [x] Verification commands
- [x] Troubleshooting table
- [x] Commit

## Task 11: docs/root-files.md

- [x] Every root-level file and directory documented
- [x] Expanded sections for Dockerfile, docker-compose.yml, Makefile, .golangci.yml, .mcp.json
- [x] Removed reference to deleted cd.yml
- [x] Commit

## Task 12: docs/troubleshooting.md

- [x] 10 common problems in table format (problem, error message, cause, fix)
- [x] How to debug section (MCP_LOG_LEVEL=debug, Docker logs, test API key)
- [x] Rate limiter behavior note (token bucket, burst = MCP_RATE_LIMIT)
- [x] Graceful shutdown note (120-second timeout)
- [x] Commit

## Task 13: CONTRIBUTING.md

- [x] Coding standards (naming, imports, file length, error handling)
- [x] Branch workflow
- [x] Quality gate
- [x] PR process
- [x] Links to setup-and-operations.md for prerequisites, go-guide.md for glossary
- [x] Commit

## Task 14: README.md

- [x] Concise landing page (~49 lines)
- [x] Overview, tools list, quick start
- [x] Embed high-level architecture diagram
- [x] Documentation link table (all 11 docs)
- [x] License: MIT, Copyright (c) 2026 Terence
- [x] Commit

---

## Verification

- [x] All 8 PNG diagrams render correctly
- [x] All markdown links between docs resolve
- [x] README.md is under 60 lines
- [x] No content duplicated across docs (each topic has one canonical location)
- [x] All original README content preserved in new docs
- [x] LICENSE reference correct (MIT, Copyright (c) 2026 Terence)
- [x] .env.example has inline comments for every variable
- [x] Every root-level file documented in root-files.md
- [x] cd.yml references removed (file was deleted)
- [x] Auth is optional in HTTP mode (code updated, docs reflect this)
- [x] Per-user tokens file with hot-reload documented and tested
