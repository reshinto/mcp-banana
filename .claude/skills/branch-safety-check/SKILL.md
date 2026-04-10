---
name: branch-safety-check
description: Verify the current git branch is a unique task-related working branch and create one if needed
user-invocable: true
---

# Branch Safety Check

## Task

Ensure work happens on a proper feature branch, never directly on main.

## Check Procedure

1. Get current branch: `git branch --show-current`
2. If on `main` or `master`:
   - Determine appropriate branch name from the current task
   - Format: `<type>/<short-description>` (e.g., `feat/user-auth`, `fix/login-error`)
   - Types: feat, fix, refactor, test, docs, chore
   - Create and switch: `git checkout -b <branch-name>`
3. If already on a feature branch:
   - Verify it's not stale (check for uncommitted changes)
   - Report current branch name

## Safety Rules

- Never commit directly to main/master
- Branch names must be descriptive of the task
- No AI/assistant/generated references in branch names
- If unsure about branch name, ask the user
