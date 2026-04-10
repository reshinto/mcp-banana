#!/usr/bin/env bash
# Unified session-end quality gate. Runs ONCE as the absolute last step.
#
# Design:
#   - Per-step timeouts via `timeout` command
#   - Output suppressed on success, capped at 30 lines on failure
#   - Accumulates all failures before exiting
#   - E2E and Storybook sections are conditional (skipped when command is empty)
#   - Coverage advisory thresholds checked after test run
#   - trap cleanup kills orphaned child processes
#   - Always exits 0 (advisory gate — does not hard-block the shell)

set -uo pipefail  # NOT set -e — we accumulate failures

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
cd "$PROJECT_DIR"
FAILED=0

# Commands injected at scaffold time
LINT_CMD="golangci-lint run"
FORMAT_CMD="gofmt -w ."
TYPECHECK_CMD="go vet ./..."
TEST_CMD="go test -race -coverprofile=coverage.out ./..."
E2E_CMD=""
STORYBOOK_BUILD_CMD=""

# Coverage thresholds (advisory, non-blocking)
COVERAGE_STATEMENTS="80"
COVERAGE_BRANCHES="75"
COVERAGE_FUNCTIONS="80"
COVERAGE_LINES="80"

# Cleanup orphaned child processes on exit
cleanup() { jobs -p 2>/dev/null | xargs -r kill 2>/dev/null || true; }
trap cleanup EXIT

# Collect changed files for conditional checks
CHANGED=$(git diff --name-only HEAD 2>/dev/null || true)
STAGED=$(git diff --cached --name-only 2>/dev/null || true)
UNTRACKED=$(git ls-files --others --exclude-standard 2>/dev/null || true)
ALL_CHANGES=$(printf '%s\n%s\n%s\n' "$CHANGED" "$STAGED" "$UNTRACKED" | sort -u | grep -v '^$' || true)

# Exit early if no changes at all
if [ -z "$ALL_CHANGES" ]; then
  echo "Quality gate: no changes detected, skipping." >&2
  exit 0
fi

SRC_CHANGES=$(echo "$ALL_CHANGES" | grep -cE '^src/' || true)
UI_CHANGES=$(echo "$ALL_CHANGES" | grep -cE '\.(tsx|css|html)$|^e2e/' || true)
STORY_CHANGES=$(echo "$ALL_CHANGES" | grep -cE '\.(tsx|stories\.tsx)$' || true)
CLAUDE_CHANGES=$(echo "$ALL_CHANGES" | grep -cE '^\.claude/' || true)
DOC_TRIGGER=$(echo "$ALL_CHANGES" | grep -cE '^src/|^docs/|^README|package\.json|\.github/' || true)

run_step() {
  local step_name="$1" cmd="$2" step_timeout="${3:-120}"
  echo "--- $step_name ---" >&2
  local outfile="/tmp/gate-${step_name}-$$.txt"
  if ! timeout "$step_timeout" bash -c "$cmd" > "$outfile" 2>&1; then
    echo "FAILED: $step_name" >&2
    tail -30 "$outfile" >&2
    FAILED=1
  else
    echo "PASSED: $step_name" >&2
  fi
  rm -f "$outfile"
}

echo "=== Unified Quality Gate ===" >&2

# === BLOCKING STEPS (all run, accumulate failures) ===

run_step "lint" "$LINT_CMD" 60
run_step "format" "$FORMAT_CMD" 30
run_step "typecheck" "$TYPECHECK_CMD" 60

# Tests with coverage (single run)
run_step "tests" "$TEST_CMD" 120

# Conditional: Storybook build only if command is configured and story files changed
if [ -n "$STORYBOOK_BUILD_CMD" ]; then
  if [ "$STORY_CHANGES" -gt 0 ]; then
    run_step "storybook" "$STORYBOOK_BUILD_CMD" 300
  else
    echo "--- storybook --- SKIPPED (no story file changes)" >&2
  fi
fi

# Conditional: E2E only if command is configured and UI/E2E files changed
if [ -n "$E2E_CMD" ]; then
  if [ "$UI_CHANGES" -gt 0 ]; then
    run_step "e2e" "$E2E_CMD" 300
  else
    echo "--- e2e --- SKIPPED (no UI/E2E file changes)" >&2
  fi
fi

# Security: unsafe patterns in src/ (blocking)
# Pattern file avoids triggering security linting hooks on the script itself
if [ "$SRC_CHANGES" -gt 0 ]; then
  SECURITY_PATTERNS="$PROJECT_DIR/.claude/hooks/security-patterns.txt"
  if [ -f "$SECURITY_PATTERNS" ]; then
    UNSAFE=$(grep -rnEf "$SECURITY_PATTERNS" src/ \
      --include='*.ts' --include='*.tsx' \
      | grep -v '^\s*//' \
      | grep -v 'node_modules' \
      || true)
    if [ -n "$UNSAFE" ]; then
      echo "--- security-patterns --- FAILED" >&2
      echo "$UNSAFE" | head -10 >&2
      FAILED=1
    else
      echo "--- security-patterns --- PASSED" >&2
    fi
  else
    echo "--- security-patterns --- SKIPPED (patterns file missing)" >&2
  fi
else
  echo "--- security-patterns --- SKIPPED (no src/ changes)" >&2
fi

# === ADVISORY CHECKS (always continue, print warnings) ===

echo "" >&2
echo "=== Advisory Checks ===" >&2

# npm audit (non-blocking, only when package.json exists)
if [ -f "package.json" ]; then
  npm audit --audit-level=high --omit=dev 2>&1 | tail -5 >&2 || echo "WARN: npm audit found issues" >&2
fi

# Coverage thresholds advisory (non-blocking, uses awk not bc)
# Checks thresholds only when values are configured (non-empty)
if [ -n "$COVERAGE_STATEMENTS" ] || [ -n "$COVERAGE_BRANCHES" ] || [ -n "$COVERAGE_FUNCTIONS" ] || [ -n "$COVERAGE_LINES" ]; then
  COVERAGE_LOG="/tmp/gate-tests-$$.txt"
  if [ -f "$COVERAGE_LOG" ]; then
    check_coverage() {
      local metric_name="$1" threshold="$2"
      [ -z "$threshold" ] && return
      local value
      value=$(grep -oE "${metric_name}\s*:\s*[0-9.]+" "$COVERAGE_LOG" | grep -oE '[0-9.]+' | head -1 || echo "0")
      if [ -n "$value" ] && [ "$value" != "0" ]; then
        local below
        below=$(awk "BEGIN { print ($value < $threshold) }")
        if [ "$below" = "1" ]; then
          echo "WARN: ${metric_name} coverage ${value}% < ${threshold}% threshold" >&2
        fi
      fi
    }
    check_coverage "Statements" "$COVERAGE_STATEMENTS"
    check_coverage "Branches" "$COVERAGE_BRANCHES"
    check_coverage "Functions" "$COVERAGE_FUNCTIONS"
    check_coverage "Lines" "$COVERAGE_LINES"
  fi
fi

# Docs advisory (non-blocking)
if [ "$DOC_TRIGGER" -gt 0 ]; then
  README_UPDATED=$(echo "$ALL_CHANGES" | grep -c '^README.md$' || true)
  DOCS_UPDATED=$(echo "$ALL_CHANGES" | grep -c '^docs/' || true)
  if [ "$README_UPDATED" -eq 0 ] && [ "$DOCS_UPDATED" -eq 0 ]; then
    echo "WARN: Source/config changes detected but no docs updated — check if docs need updating" >&2
  fi
fi

# Claude system config advisory (non-blocking)
if [ "$CLAUDE_CHANGES" -gt 0 ]; then
  echo "INFO: .claude/ config files changed — verify hooks, skills, and agents are consistent" >&2
fi

echo "" >&2
echo "=== Quality Gate Complete ===" >&2

if [ "$FAILED" -ne 0 ]; then
  echo "QUALITY GATE FAILED — fix issues above before pushing" >&2
  # Advisory only: exit 0 so the shell is not hard-blocked.
  # Claude Code reads FAILED status from the output above.
  exit 0
fi
echo "All checks passed." >&2
exit 0
