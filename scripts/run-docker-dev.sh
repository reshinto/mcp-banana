#!/usr/bin/env bash
# Run mcp-banana in Docker development mode (localhost only, no TLS).
# Usage: ./scripts/run-docker-dev.sh
#
# Prerequisites:
#   - Docker and Docker Compose installed
#   - .env file (GEMINI_API_KEY optional if clients send X-Gemini-API-Key header)
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
  echo "NOTE: GEMINI_API_KEY is not set in .env. Clients must provide their own key via X-Gemini-API-Key header." >&2
fi

echo "Building and starting mcp-banana (dev mode, 127.0.0.1:8847)..."
docker compose up -d --build

echo ""
echo "Server running at http://127.0.0.1:8847"
echo "Health check: curl http://127.0.0.1:8847/healthz"
echo "Logs: docker compose logs -f"
echo "Stop: docker compose down"
