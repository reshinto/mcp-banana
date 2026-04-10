#!/usr/bin/env bash
# Run mcp-banana locally in stdio mode (for Claude Code direct integration).
# Usage: ./scripts/run-local.sh
#
# Prerequisites:
#   - Go 1.25+ installed
#   - GEMINI_API_KEY set in .env or exported in shell (optional if clients
#     provide their own key via X-Gemini-API-Key header)
#
# This script loads .env, builds the binary, and starts in stdio mode.

set -euo pipefail
cd "$(dirname "$0")/.."

# Load .env if it exists
if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

if [ -z "${GEMINI_API_KEY:-}" ]; then
  echo "NOTE: GEMINI_API_KEY is not set. Clients must provide their own key via X-Gemini-API-Key header." >&2
fi

echo "Building mcp-banana..."
make build

echo "Starting mcp-banana in stdio mode..."
./mcp-banana --transport stdio
