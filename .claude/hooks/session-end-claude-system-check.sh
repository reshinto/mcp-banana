#!/usr/bin/env bash
# Session end hook: validate .claude/ system configuration consistency.
# Checks agent/skill frontmatter, hook script references, and directory listings.
# Non-zero exit blocks git operations on broken config.

set -euo pipefail

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
cd "$PROJECT_DIR"

# Only run when .claude/ files changed
CHANGED=$(git diff --name-only HEAD 2>/dev/null || true)
STAGED=$(git diff --cached --name-only 2>/dev/null || true)
UNTRACKED=$(git ls-files --others --exclude-standard 2>/dev/null || true)
ALL=$(printf '%s\n%s\n%s\n' "$CHANGED" "$STAGED" "$UNTRACKED" | sort -u | grep -v '^$' || true)

CLAUDE_CHANGES=$(echo "$ALL" | grep -cE '^\.claude/' || true)

if [ "$CLAUDE_CHANGES" -eq 0 ]; then
  echo "System config: No .claude/ changes detected, skipping." >&2
  exit 0
fi

FAILED=0

echo "=== Claude System Config Check ===" >&2

# Check 1: Agent frontmatter validation
echo "Validating agent definitions..." >&2
for AGENT_FILE in .claude/agents/*.md; do
  if [ ! -f "$AGENT_FILE" ]; then continue; fi

  AGENT_NAME=$(basename "$AGENT_FILE")
  FRONTMATTER=$(head -20 "$AGENT_FILE")

  MISSING_KEYS=""
  for KEY in "name:" "description:" "tools:" "model:" "maxTurns:"; do
    if ! echo "$FRONTMATTER" | grep -q "$KEY"; then
      MISSING_KEYS="${MISSING_KEYS} ${KEY}"
    fi
  done

  if [ -n "$MISSING_KEYS" ]; then
    echo "FAIL: Agent ${AGENT_NAME} missing frontmatter:${MISSING_KEYS}" >&2
    FAILED=1
  fi
done
if [ "$FAILED" -eq 0 ]; then
  echo "PASS: All agent frontmatter valid" >&2
fi

# Check 2: Skill frontmatter validation
SKILL_FAILED=0
echo "Validating skill definitions..." >&2
for SKILL_FILE in .claude/skills/*/SKILL.md; do
  if [ ! -f "$SKILL_FILE" ]; then continue; fi

  SKILL_DIR=$(dirname "$SKILL_FILE" | xargs basename)
  FRONTMATTER=$(head -10 "$SKILL_FILE")

  MISSING_KEYS=""
  for KEY in "name:" "description:"; do
    if ! echo "$FRONTMATTER" | grep -q "$KEY"; then
      MISSING_KEYS="${MISSING_KEYS} ${KEY}"
    fi
  done

  if [ -n "$MISSING_KEYS" ]; then
    echo "FAIL: Skill ${SKILL_DIR} missing frontmatter:${MISSING_KEYS}" >&2
    SKILL_FAILED=1
    FAILED=1
  fi
done
if [ "$SKILL_FAILED" -eq 0 ]; then
  echo "PASS: All skill frontmatter valid" >&2
fi

# Check 3: settings.json hook script references exist
HOOK_FAILED=0
echo "Validating hook script references..." >&2
HOOK_SCRIPTS=$(grep -oE '\.claude/hooks/[a-z0-9_-]+\.sh' .claude/settings.json 2>/dev/null | sort -u || true)
for HOOK_PATH in $HOOK_SCRIPTS; do
  if [ ! -f "$HOOK_PATH" ]; then
    echo "FAIL: Hook script referenced in settings.json not found: ${HOOK_PATH}" >&2
    HOOK_FAILED=1
    FAILED=1
  elif [ ! -x "$HOOK_PATH" ]; then
    echo "FAIL: Hook script not executable: ${HOOK_PATH}" >&2
    HOOK_FAILED=1
    FAILED=1
  fi
done
if [ "$HOOK_FAILED" -eq 0 ]; then
  echo "PASS: All hook script references valid" >&2
fi

# Check 4: Warn about orphaned hook scripts (in hooks/ but not in settings.json)
for SCRIPT in .claude/hooks/*.sh; do
  if [ ! -f "$SCRIPT" ]; then continue; fi
  SCRIPT_NAME=$(basename "$SCRIPT")
  if ! grep -q "$SCRIPT_NAME" .claude/settings.json 2>/dev/null; then
    echo "WARN: Hook script not referenced in settings.json: ${SCRIPT_NAME}" >&2
  fi
done

echo "=== Claude System Config Check Complete ===" >&2

if [ "$FAILED" -ne 0 ]; then
  echo "BLOCKED: System config issues found. Fix before git operations." >&2
  exit 1
fi

echo "All system config checks passed." >&2
exit 0
