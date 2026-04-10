---
name: verification
description: Pre-completion verification with feature completeness checks, coverage validation, branch safety, and state management review
user-invocable: true
---

# Verification

## Task

Verify all work is complete and correct before claiming done, committing, or creating a PR.

## Steps

### 1. Feature Completeness (if a feature was added or changed)

- [ ] Core implementation exists and is reachable from the application entry point
- [ ] All required variants or configurations are implemented
- [ ] Feature is registered or wired into the project's established integration point
- [ ] Unit tests pass
- [ ] Integration tests pass (if applicable)
- [ ] Component story or integration test exists in the feature's `__tests__/` directory
- [ ] E2E coverage follows project-specific spec conventions in `.claude/rules/`

### 2. Coverage Check

Run `go test ./...` with coverage and verify thresholds per `.claude/rules/` testing rules.

### 3. Branch Safety

- Confirm not on `main` or `master`
- Confirm branch name follows `<type>/<description>` convention

### 4. State Management Check

- No unintended persistence of transient state (localStorage, URL params, global variables)
- State resets correctly on context or view switch
- No state leaks between independent features or sessions

## Rules

- Run ALL verification steps — do not skip any
- Evidence before assertions — show command output, not just "tests pass"
- If any step fails, fix before proceeding
- The full quality gate (lint, format, typecheck, test, build) runs automatically at session end — no need to duplicate it here
