#!/usr/bin/env bash
# PreToolUse hook: block git commit and git push on main/master branches.
# Ensures all work happens on task-related feature branches.

set -euo pipefail

INPUT=$(cat)

COMMAND=$(echo "$INPUT" | grep -o '"command":"[^"]*"' | head -1 | sed 's/"command":"//;s/"$//' || true)

if [ -z "$COMMAND" ]; then
  exit 0
fi

# Only check git commit and git push commands
IS_GIT_COMMIT=$(echo "$COMMAND" | grep -ci 'git commit' || true)
IS_GIT_PUSH=$(echo "$COMMAND" | grep -ci 'git push' || true)

if [ "$IS_GIT_COMMIT" -eq 0 ] && [ "$IS_GIT_PUSH" -eq 0 ]; then
  exit 0
fi

CURRENT_BRANCH=$(git branch --show-current 2>/dev/null || echo "")

if [ "$CURRENT_BRANCH" = "main" ] || [ "$CURRENT_BRANCH" = "master" ]; then
  echo "BLOCKED: Cannot commit or push directly to '$CURRENT_BRANCH'." >&2
  echo "Create a feature branch first: git checkout -b <type>/<short-description>" >&2
  exit 2
fi

exit 0
