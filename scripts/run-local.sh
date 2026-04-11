#!/usr/bin/env bash
# Run mcp-banana locally in stdio mode (for Claude Code direct integration).
# Usage: ./scripts/run-local.sh
#
# Prerequisites:
#   - Go 1.25+ installed
#
# This script loads .env, builds the binary, and starts in stdio mode.
# A credentials.json file is auto-created in the project root on first run.

set -euo pipefail
cd "$(dirname "$0")/.."

# Load .env if it exists
if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

echo "Building mcp-banana..."
make build

echo "Starting mcp-banana in stdio mode..."
./mcp-banana --transport stdio
