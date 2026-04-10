#!/usr/bin/env bash
# Run mcp-banana in Docker production mode (public IP, TLS, OAuth).
# Usage: ./scripts/run-docker-prod.sh
#
# Prerequisites:
#   - Docker and Docker Compose installed
#   - .env file with MCP_AUTH_TOKEN, TLS and OAuth vars configured
#     (GEMINI_API_KEY optional if clients send X-Gemini-API-Key header)
#   - TLS certificates at /etc/letsencrypt/live/mcp.terencekong.net/
#   - DNS A record for mcp.terencekong.net pointing to this server
#
# The server binds to 0.0.0.0:8847 (all interfaces) with TLS.

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

# Check TLS cert existence
CERT_DIR="/etc/letsencrypt/live/mcp.terencekong.net"
if [ ! -d "$CERT_DIR" ]; then
  echo "WARNING: TLS certificate directory not found at $CERT_DIR" >&2
  echo "  Generate certs with: certbot certonly --manual --preferred-challenges dns -d mcp.terencekong.net" >&2
  echo "  Continuing without TLS verification..." >&2
fi

echo "Building and starting mcp-banana (production mode, 0.0.0.0:8847)..."
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build

echo ""
echo "Server running at https://mcp.terencekong.net:8847"
echo "Health check: curl -k https://mcp.terencekong.net:8847/healthz"
echo "Logs: docker compose logs -f"
echo "Stop: docker compose -f docker-compose.yml -f docker-compose.prod.yml down"
