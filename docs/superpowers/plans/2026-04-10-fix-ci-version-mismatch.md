# Fix: CI golangci-lint Go Version Mismatch

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix GitHub Actions CI failure caused by Go version mismatch between go.mod (1.26.1) and CI toolchain (Go 1.24, golangci-lint v2.1.6).

**Architecture:** Update CI workflow and documentation to match local toolchain versions.

**Tech Stack:** GitHub Actions, Go 1.26, golangci-lint v2.11.4

**Status:** COMPLETE

---

## Context

GitHub Actions CI fails with:
```
Error: can't load config: the Go language version (go1.24) used to build golangci-lint
is lower than the targeted Go version (1.26.1)
```

Root cause: `go.mod` was initialized with Go 1.26.1 (the local machine's version), but CI installs Go 1.24 and golangci-lint v2.1.6 (built with Go 1.24). golangci-lint refuses to analyze code targeting a newer Go version than it was compiled with.

| | Local | CI (broken) | CI (fixed) |
|---|---|---|---|
| Go | 1.26.1 | 1.24 | 1.26 |
| golangci-lint | v2.11.4 | v2.1.6 | v2.11.4 |

---

## Task 1: Update `.github/workflows/ci.yml`

- [x] Update `go-version: '1.24'` to `go-version: '1.26'`
- [x] Update golangci-lint `version: v2.1.6` to `version: v2.11.4`
- [x] Commit

## Task 2: Update documentation references

- [x] `docs/setup-and-operations.md` -- update Go 1.24 -> 1.26, golangci-lint v2.1.6 -> v2.11.4
- [x] `docs/troubleshooting.md` -- update version references
- [x] Commit

## Task 3: Save plan and push

- [x] Save this plan to `docs/superpowers/plans/2026-04-10-fix-ci-version-mismatch.md`
- [x] Commit and push

## Verification

- [x] `golangci-lint run` passes locally
- [x] Push to branch, verify CI passes on GitHub Actions
