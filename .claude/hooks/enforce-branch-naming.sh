#!/usr/bin/env bash
# PreToolUse hook: enforce branch naming convention on branch creation.
# Blocks if type prefix is invalid.
# Reads allowed types from ${CLAUDE_PROJECT_DIR}/.claude/hooks/branch-types.json if present,
# otherwise falls back to a sensible default set.

set -euo pipefail

INPUT=$(cat)

# Extract the command being run
COMMAND=$(echo "$INPUT" | grep -o '"command":"[^"]*"' | head -1 | sed 's/"command":"//;s/"$//' || true)
if [ -z "$COMMAND" ]; then
  exit 0
fi

# Extract branch name from various git commands
BRANCH_NAME=""
if echo "$COMMAND" | grep -qE 'git checkout -[bB] '; then
  BRANCH_NAME=$(echo "$COMMAND" | sed -E 's/.*git checkout -[bB] +([^ ]+).*/\1/')
elif echo "$COMMAND" | grep -qE 'git switch -c '; then
  BRANCH_NAME=$(echo "$COMMAND" | sed -E 's/.*git switch -c +([^ ]+).*/\1/')
elif echo "$COMMAND" | grep -qE 'git branch [^-]'; then
  BRANCH_NAME=$(echo "$COMMAND" | sed -E 's/.*git branch +([^ ]+).*/\1/')
fi

if [ -z "$BRANCH_NAME" ]; then
  exit 0
fi

# Load allowed types from project config if available, otherwise use defaults
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
BRANCH_TYPES_JSON="$PROJECT_DIR/.claude/hooks/branch-types.json"

DEFAULT_TYPES="feat fix chore refactor docs test ci"

if [ -f "$BRANCH_TYPES_JSON" ]; then
  # Extract types from JSON array: ["feat", "fix", ...] → "feat fix ..."
  VALID_TYPES=$(grep -o '"[a-zA-Z_-]*"' "$BRANCH_TYPES_JSON" 2>/dev/null | tr -d '"' | tr '\n' ' ' | sed 's/ *$//' || echo "")

  if [ -z "$VALID_TYPES" ]; then
    VALID_TYPES="$DEFAULT_TYPES"
  fi
else
  VALID_TYPES="$DEFAULT_TYPES"
fi

TYPE=$(echo "$BRANCH_NAME" | cut -d'/' -f1)

if ! echo "$BRANCH_NAME" | grep -q '/'; then
  echo "BLOCKED: Branch name '$BRANCH_NAME' must follow convention: <type>/<description>" >&2
  echo "Valid types: $VALID_TYPES" >&2
  echo "Examples: feat/add-login, fix/auth-redirect, chore/update-deps" >&2
  exit 2
fi

VALID=false
for VTYPE in $VALID_TYPES; do
  if [ "$TYPE" = "$VTYPE" ]; then
    VALID=true
    break
  fi
done

if [ "$VALID" = "false" ]; then
  echo "BLOCKED: Invalid branch type '$TYPE' in '$BRANCH_NAME'" >&2
  echo "Valid types: $VALID_TYPES" >&2
  exit 2
fi

exit 0
