#!/usr/bin/env bash
# Advisory naming convention check on staged Go files.
# Catches common forms of banned abbreviated variables.
#
# LIMITATION: This uses regex, not a Go parser. It will catch most
# `:=` declarations but may miss some forms (e.g., function parameters)
# and may false-positive on string literals or comments. The authoritative
# rule is in .claude/rules/coding-standards.md -- this hook is a safety net.

STAGED_GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$')
if [ -z "$STAGED_GO_FILES" ]; then
  exit 0
fi

# Banned abbreviations in short-variable declarations (:=)
# NOTE: err, ctx, req, resp, cfg, srv are ALLOWED (standard Go idioms).
# This catches non-standard abbreviations only.
ABBREV_PATTERN='\b(mw|msg|sig|idx|num|val|buf|tmp|fn|cb|ch|opts|addr)\b\s*:='
VIOLATIONS=""

for FILE in $STAGED_GO_FILES; do
  # Skip test helper files and generated code
  if echo "$FILE" | grep -q '_generated\.go$'; then
    continue
  fi
  MATCHES=$(grep -nP "$ABBREV_PATTERN" "$FILE" 2>/dev/null || true)
  if [ -n "$MATCHES" ]; then
    VIOLATIONS="$VIOLATIONS\n$FILE:\n$MATCHES\n"
  fi
done

if [ -n "$VIOLATIONS" ]; then
  echo "NAMING ADVISORY: Possible abbreviated variable names in staged files:"
  echo -e "$VIOLATIONS"
  echo ""
  echo "Project convention requires full words (except allowed Go idioms: err, ctx, req, resp, cfg, srv, test)."
  echo "See .claude/rules/coding-standards.md for the complete naming policy."
  echo ""
  echo "If these are false positives (e.g., inside string literals), the commit can still proceed."
  exit 1
fi
exit 0
