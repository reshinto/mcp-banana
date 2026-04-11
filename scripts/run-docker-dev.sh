#!/usr/bin/env bash
# Run mcp-banana in Docker development mode (localhost only, no TLS).
# Usage: ./scripts/run-docker-dev.sh
#
# Compatible with:
#   - Docker Compose V2 plugin (docker compose)
#   - Docker Compose V1 standalone (docker-compose)
#   - Docker 19.03+
#
# Prerequisites:
#   - Docker and Docker Compose installed
#   - .env file configured
#
# The server binds to 127.0.0.1:8847 (loopback only).
# Access it from the same machine or via SSH tunnel.

set -euo pipefail
cd "$(dirname "$0")/.."

# Detect docker compose command (V2 plugin or V1 standalone)
COMPOSE_CMD=""
if docker compose version &>/dev/null; then
  COMPOSE_CMD="docker compose"
elif command -v docker-compose &>/dev/null; then
  COMPOSE_CMD="docker-compose"
else
  echo "ERROR: Docker Compose is not installed." >&2
  echo "  Docker Compose V2: sudo apt-get install -y docker-compose-plugin" >&2
  echo "  Docker Compose V1: sudo apt-get install -y docker-compose" >&2
  exit 1
fi

if [ ! -f .env ]; then
  echo "ERROR: .env file not found. Copy .env.example and configure it:" >&2
  echo "  cp .env.example .env" >&2
  exit 1
fi

# Create credentials.json if it doesn't exist (mounted as a Docker volume)
if [ ! -f credentials.json ]; then
  echo "{}" > credentials.json
  chmod 600 credentials.json
  echo "Created credentials.json"
fi

# Stop any existing container to free port 8847
${COMPOSE_CMD} down 2>/dev/null || true

echo "Building and starting mcp-banana (dev mode, 127.0.0.1:8847)..."
${COMPOSE_CMD} up -d --build

echo ""
echo "Server running at http://127.0.0.1:8847"
echo "Health check: curl http://127.0.0.1:8847/healthz"
echo "Logs: ${COMPOSE_CMD} logs -f"
echo "Stop: ${COMPOSE_CMD} down"
