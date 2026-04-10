#!/usr/bin/env bash
# Run mcp-banana in Docker development mode (localhost only, no TLS).
# Usage: ./scripts/run-docker-dev.sh
#
# Prerequisites:
#   - Docker and Docker Compose installed
#   - .env file with at least GEMINI_API_KEY set
#
# The server binds to 127.0.0.1:8847 (loopback only).
# Access it from the same machine or via SSH tunnel.

set -euo pipefail
cd "$(dirname "$0")/.."

if [ ! -f .env ]; then
  echo "ERROR: .env file not found. Copy .env.example and configure it:" >&2
  echo "  cp .env.example .env" >&2
  exit 1
fi

if ! grep -q "^GEMINI_API_KEY=.\+" .env 2>/dev/null; then
  echo "ERROR: GEMINI_API_KEY is not set in .env" >&2
  exit 1
fi

echo "Building and starting mcp-banana (dev mode, 127.0.0.1:8847)..."
docker compose up -d --build

echo ""
echo "Server running at http://127.0.0.1:8847"
echo "Health check: curl http://127.0.0.1:8847/healthz"
echo "Logs: docker compose logs -f"
echo "Stop: docker compose down"
