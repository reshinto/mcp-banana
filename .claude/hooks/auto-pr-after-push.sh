#!/usr/bin/env bash
# PostToolUse hook: remind to create a PR after a successful git push.
# Only triggers on feature branches (not main/master) when no PR exists yet.

set -euo pipefail

INPUT=$(cat)

COMMAND=$(echo "$INPUT" | grep -o '"command":"[^"]*"' | head -1 | sed 's/"command":"//;s/"$//' || true)

if [ -z "$COMMAND" ]; then
  exit 0
fi

IS_GIT_PUSH=$(echo "$COMMAND" | grep -ci 'git push' || true)

if [ "$IS_GIT_PUSH" -eq 0 ]; then
  exit 0
fi

CURRENT_BRANCH=$(git branch --show-current 2>/dev/null || echo "")

if [ "$CURRENT_BRANCH" = "main" ] || [ "$CURRENT_BRANCH" = "master" ] || [ -z "$CURRENT_BRANCH" ]; then
  exit 0
fi

# Check if a PR already exists for this branch
EXISTING_PR=$(gh pr list --head "$CURRENT_BRANCH" --state open --json number --jq '.[0].number' 2>/dev/null || echo "")

if [ -n "$EXISTING_PR" ]; then
  echo "PR #$EXISTING_PR already exists for branch '$CURRENT_BRANCH'." >&2
  exit 0
fi

echo "REMINDER: Branch '$CURRENT_BRANCH' was pushed but has no open PR. Create one now with: gh pr create" >&2
exit 0
