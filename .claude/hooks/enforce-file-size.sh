#!/usr/bin/env bash
# Advisory file-size check on staged Go files.
# Flags files exceeding 500 lines to encourage modular, focused files.
#
# The 500-line limit enforces: one responsibility per file, short functions,
# DRY principle, and easy-to-review code. Files that grow beyond this should
# be split into focused sub-files.

STAGED_GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$')
if [ -z "$STAGED_GO_FILES" ]; then
  exit 0
fi

MAX_LINES=500
VIOLATIONS=""

for FILE in $STAGED_GO_FILES; do
  LINE_COUNT=$(wc -l < "$FILE" | tr -d ' ')
  if [ "$LINE_COUNT" -gt "$MAX_LINES" ]; then
    VIOLATIONS="$VIOLATIONS\n  $FILE: $LINE_COUNT lines (max $MAX_LINES)"
  fi
done

if [ -n "$VIOLATIONS" ]; then
  echo "FILE SIZE ADVISORY: The following files exceed $MAX_LINES lines:"
  echo -e "$VIOLATIONS"
  echo ""
  echo "Split large files into focused sub-files with one responsibility each."
  echo "See .claude/rules/coding-standards.md for the modularity policy."
  exit 1
fi
exit 0
