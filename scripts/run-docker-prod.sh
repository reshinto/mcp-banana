#!/usr/bin/env bash
# Run mcp-banana in Docker production mode (public IP, TLS, OAuth).
# Usage: ./scripts/run-docker-prod.sh
#
# Prerequisites:
#   - Docker and Docker Compose installed
#   - .env file with MCP_DOMAIN, TLS, and optionally OAuth vars configured
#     (GEMINI_API_KEY optional if clients send X-Gemini-API-Key header)
#   - TLS certificates at /etc/letsencrypt/live/<MCP_DOMAIN>/
#   - DNS A record for <MCP_DOMAIN> pointing to this server
#
# The server binds to 0.0.0.0:8847 (all interfaces) with TLS.

set -euo pipefail
cd "$(dirname "$0")/.."

if [ ! -f .env ]; then
  echo "ERROR: .env file not found. Copy .env.example and configure it:" >&2
  echo "  cp .env.example .env" >&2
  exit 1
fi

# Load .env to read MCP_DOMAIN
set -a
# shellcheck disable=SC1091
source .env
set +a

DOMAIN="${MCP_DOMAIN:-}"
if [ -z "$DOMAIN" ]; then
  echo "ERROR: MCP_DOMAIN is not set in .env" >&2
  echo "  Example: MCP_DOMAIN=mcp.yourdomain.com" >&2
  exit 1
fi

# Auto-update OAUTH_BASE_URL in .env from MCP_DOMAIN if empty
if grep -q "^OAUTH_BASE_URL=$" .env 2>/dev/null; then
  sed -i.bak "s|^OAUTH_BASE_URL=$|OAUTH_BASE_URL=https://${DOMAIN}:8847|" .env && rm -f .env.bak
  echo "OAUTH_BASE_URL set to https://${DOMAIN}:8847 in .env"
fi

# Auto-update MCP_TLS_CERT_FILE and MCP_TLS_KEY_FILE in .env if empty
if grep -q "^MCP_TLS_CERT_FILE=$" .env 2>/dev/null; then
  sed -i.bak "s|^MCP_TLS_CERT_FILE=$|MCP_TLS_CERT_FILE=/certs/fullchain.pem|" .env && rm -f .env.bak
  sed -i.bak "s|^MCP_TLS_KEY_FILE=$|MCP_TLS_KEY_FILE=/certs/privkey.pem|" .env && rm -f .env.bak
  echo "MCP_TLS_CERT_FILE and MCP_TLS_KEY_FILE set to /certs/ paths in .env"
fi

# Re-source .env after updates
set -a
# shellcheck disable=SC1091
source .env
set +a

if ! grep -q "^GEMINI_API_KEY=.\+" .env 2>/dev/null; then
  echo "NOTE: GEMINI_API_KEY is not set in .env. Clients must provide their own key via X-Gemini-API-Key header." >&2
fi

# Check TLS cert existence
CERT_DIR="/etc/letsencrypt/live/${DOMAIN}"
if [ ! -d "$CERT_DIR" ]; then
  echo "WARNING: TLS certificate directory not found at $CERT_DIR" >&2
  echo "  Generate certs with: certbot certonly --manual --preferred-challenges dns -d ${DOMAIN}" >&2
  echo "  Continuing without TLS verification..." >&2
fi

echo "Building and starting mcp-banana (production mode, 0.0.0.0:8847, domain: ${DOMAIN})..."
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build

echo ""
echo "Server running at https://${DOMAIN}:8847"
echo "Health check: curl -k https://${DOMAIN}:8847/healthz"
echo "Logs: docker compose logs -f"
echo "Stop: docker compose -f docker-compose.yml -f docker-compose.prod.yml down"
