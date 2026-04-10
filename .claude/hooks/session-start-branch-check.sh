#!/usr/bin/env bash
# Session start hook: warn if working directly on main/master.
# Also validates branch naming convention.

set -euo pipefail

CURRENT_BRANCH=$(git branch --show-current 2>/dev/null || echo "")

if [ -z "$CURRENT_BRANCH" ]; then
  echo "WARNING: Not in a git repository or no branch checked out." >&2
  exit 0
fi

PROTECTED_BRANCHES="main master"

for PROTECTED in $PROTECTED_BRANCHES; do
  if [ "$CURRENT_BRANCH" = "$PROTECTED" ]; then
    echo "BLOCKED: Currently on '$CURRENT_BRANCH'. You must create a feature branch before making changes." >&2
    echo "Run: git checkout -b <type>/<description>" >&2
    echo "Examples: feat/add-login, fix/auth-redirect, chore/update-deps" >&2
    exit 2
  fi
done

# Validate branch naming convention (warn, not block)
VALID_TYPES="feat fix chore refactor docs test ci"
TYPE=$(echo "$CURRENT_BRANCH" | cut -d'/' -f1)

if ! echo "$CURRENT_BRANCH" | grep -q '/'; then
  echo "WARNING: Branch '$CURRENT_BRANCH' does not follow convention: <type>/<description>" >&2
  echo "Valid types: $VALID_TYPES" >&2
fi

VALID_TYPE=false
for VTYPE in $VALID_TYPES; do
  if [ "$TYPE" = "$VTYPE" ]; then
    VALID_TYPE=true
    break
  fi
done

if [ "$VALID_TYPE" = "false" ] && echo "$CURRENT_BRANCH" | grep -q '/'; then
  echo "WARNING: Unknown branch type '$TYPE' in '$CURRENT_BRANCH'" >&2
  echo "Valid types: $VALID_TYPES" >&2
fi

echo "Branch check passed: on '$CURRENT_BRANCH'" >&2
